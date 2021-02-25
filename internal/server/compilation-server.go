package server

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	pb "github.com/AlexK0/popcorn/internal/api/proto/v1"
	"github.com/AlexK0/popcorn/internal/common"
	"google.golang.org/grpc"
)

// CompilationServer ...
type CompilationServer struct {
	pb.UnimplementedCompilationServiceServer

	StartTime time.Time

	SessionsDir string
	WorkingDir  string

	GRPCServer                 *grpc.Server
	RemoteControlPassword      string
	NewPopcornServerBinaryPath string

	remoteControlLock sync.Mutex

	UserCache        *UserCache
	UploadingHeaders *SendingHeaders
	SystemHeaders    *SystemHeaderCache
	HeaderFileCache  *FileCache

	Sessions *UserSessions
}

// StartCompilationSession ...
func (s *CompilationServer) StartCompilationSession(ctx context.Context, in *pb.StartCompilationSessionRequest) (*pb.StartCompilationSessionReply, error) {
	userID := common.SHA256MessageToSHA256Struct(in.UserID)
	session := &UserSession{
		UserID:          userID,
		Compiler:        in.Compiler,
		CompilerArgs:    in.CompilerArgs,
		RequiredHeaders: make([]RequiredHeaderMetadata, 0, len(in.RequiredHeaders)),
		SourceFilePath:  in.SourceFilePath,
		FileSHA256Cache: s.UserCache.GetFilesCache(userID),
	}

	sessionID := s.Sessions.OpenNewSession(session)
	session.WorkingDir = path.Join(s.SessionsDir, fmt.Sprint(sessionID))

	if err := os.MkdirAll(session.WorkingDir, os.ModePerm); err != nil {
		s.Sessions.CloseSession(sessionID)
		return nil, fmt.Errorf("Can't create session working directory: %v", err)
	}

	missedHeadersSHA256 := make([]int32, 0, len(in.RequiredHeaders))
	missedHeadersFullCopy := make([]int32, 0, len(in.RequiredHeaders))
	for index, headerMetadata := range in.RequiredHeaders {
		headerSHA256, ok := session.FileSHA256Cache.GetFileSHA256(headerMetadata.FilePath, headerMetadata.MTime)
		session.RequiredHeaders = append(session.RequiredHeaders, RequiredHeaderMetadata{
			HeaderMetadata: headerMetadata,
			SHA256Struct:   headerSHA256,
		})
		if !ok {
			missedHeadersSHA256 = append(missedHeadersSHA256, int32(index))
			continue
		}
		if systemSHA256 := s.SystemHeaders.GetSystemHeaderSHA256(headerMetadata.FilePath); systemSHA256 == headerSHA256 {
			continue
		}
		headerPathInWorkingDir := path.Join(session.WorkingDir, headerMetadata.FilePath)
		if s.HeaderFileCache.CreateLinkFromCache(headerMetadata.FilePath, headerSHA256, headerPathInWorkingDir) {
			continue
		}
		if s.UploadingHeaders.StartHeaderSending(headerMetadata.FilePath, headerSHA256) {
			missedHeadersFullCopy = append(missedHeadersFullCopy, int32(index))
		} else {
			missedHeadersSHA256 = append(missedHeadersSHA256, int32(index))
		}
	}

	return &pb.StartCompilationSessionReply{
		SessionID:             s.Sessions.OpenNewSession(session),
		MissedHeadersSHA256:   missedHeadersSHA256,
		MissedHeadersFullCopy: missedHeadersFullCopy,
	}, nil
}

// SendHeaderSHA256 ...
func (s *CompilationServer) SendHeaderSHA256(ctx context.Context, in *pb.SendHeaderSHA256Request) (*pb.SendHeaderSHA256Reply, error) {
	session := s.Sessions.GetSession(in.SessionID)
	if session == nil {
		return nil, fmt.Errorf("Unknown SessionID %d", in.SessionID)
	}

	headerMetadata := &session.RequiredHeaders[in.HeaderIndex]
	headerMetadata.SHA256Struct = common.SHA256MessageToSHA256Struct(in.HeaderSHA256)
	session.FileSHA256Cache.SetFileSHA256(headerMetadata.FilePath, headerMetadata.MTime, headerMetadata.SHA256Struct)
	if systemSHA256 := s.SystemHeaders.GetSystemHeaderSHA256(headerMetadata.FilePath); systemSHA256 == headerMetadata.SHA256Struct {
		return &pb.SendHeaderSHA256Reply{}, nil
	}

	headerPathInWorkingDir := path.Join(session.WorkingDir, headerMetadata.FilePath)
	start := time.Now()
	// TODO Why 6 seconds?
	for time.Since(start) < 6*time.Second {
		if s.HeaderFileCache.CreateLinkFromCache(headerMetadata.FilePath, headerMetadata.SHA256Struct, headerPathInWorkingDir) {
			return &pb.SendHeaderSHA256Reply{}, nil
		}
		if s.UploadingHeaders.StartHeaderSending(headerMetadata.FilePath, headerMetadata.SHA256Struct) {
			return &pb.SendHeaderSHA256Reply{FullCopyRequired: true}, nil
		}
		// TODO Why 100 milliseconds?
		time.Sleep(100 * time.Millisecond)
	}

	s.UploadingHeaders.ForceStartHeaderSending(headerMetadata.FilePath, headerMetadata.SHA256Struct)
	return &pb.SendHeaderSHA256Reply{FullCopyRequired: true}, nil
}

// SendHeader ...
func (s *CompilationServer) SendHeader(ctx context.Context, in *pb.SendHeaderRequest) (*pb.SendHeaderReply, error) {
	session := s.Sessions.GetSession(in.SessionID)
	if session == nil {
		return nil, fmt.Errorf("Unknown SessionID %d", in.SessionID)
	}

	headerMetadata := &session.RequiredHeaders[in.HeaderIndex]
	defer s.UploadingHeaders.FinishHeaderSending(headerMetadata.FilePath, headerMetadata.SHA256Struct)
	headerPathInWorkingDir := path.Join(session.WorkingDir, headerMetadata.FilePath)
	if s.HeaderFileCache.CreateLinkFromCache(headerMetadata.FilePath, headerMetadata.SHA256Struct, headerPathInWorkingDir) {
		return &pb.SendHeaderReply{}, nil
	}

	if err := common.WriteFile(headerPathInWorkingDir, in.HeaderBody); err != nil {
		return nil, fmt.Errorf("Can't save header: %v", err)
	}

	_, _, _ = s.HeaderFileCache.SaveFileToCache(headerPathInWorkingDir, headerMetadata.FilePath, headerMetadata.SHA256Struct, int64(len(in.HeaderBody)))
	return &pb.SendHeaderReply{}, nil
}

func (s *CompilationServer) closeSession(session *UserSession, sessionID uint64, close bool) {
	if close {
		s.Sessions.CloseSession(sessionID)
		_ = os.RemoveAll(session.WorkingDir)
	}
}

// CompileSource ....
func (s *CompilationServer) CompileSource(ctx context.Context, in *pb.CompileSourceRequest) (*pb.CompileSourceReply, error) {
	session := s.Sessions.GetSession(in.SessionID)
	if session == nil {
		return nil, fmt.Errorf("Unknown SessionID %d", in.SessionID)
	}

	defer s.closeSession(session, in.SessionID, in.CloseSessionAfterBuild)

	inFileFull := path.Join(session.WorkingDir, session.SourceFilePath)
	err := common.WriteFile(inFileFull, in.SourceBody)
	if err != nil {
		return nil, fmt.Errorf("Can't write source for compilation: %v", err)
	}

	compilerArgs := make([]string, 0, 3+len(session.CompilerArgs))
	for i := 0; i < len(session.CompilerArgs); i++ {
		arg := session.CompilerArgs[i]
		if (arg == "-I" || arg == "-isystem" || arg == "-iquote") && i+1 < len(session.CompilerArgs) {
			includeDir := session.CompilerArgs[i+1]
			includeDirFull := path.Join(session.WorkingDir, includeDir)
			if _, err = os.Stat(includeDirFull); err != nil && os.IsNotExist(err) {
				i++
				continue
			}
			session.CompilerArgs[i+1] = strings.TrimLeft(includeDir, "/")
		}
		compilerArgs = append(compilerArgs, arg)
	}
	inFile := strings.TrimLeft(session.SourceFilePath, "/")
	outFile := inFile + ".o"
	compilerArgs = append(compilerArgs, inFile, "-o", outFile)

	compilerProc := exec.Command(session.Compiler, compilerArgs...)
	compilerProc.Dir = session.WorkingDir
	var compilerStderr, compilerStdout bytes.Buffer
	compilerProc.Stderr = &compilerStderr
	compilerProc.Stdout = &compilerStdout

	common.LogInfo("Launch compiler:", compilerProc.Args)
	_ = compilerProc.Run()

	outFileFull := path.Join(session.WorkingDir, outFile)
	defer os.Remove(outFileFull)

	var compiledSource []byte
	if compilerProc.ProcessState.ExitCode() == 0 {
		if compiledSource, err = ioutil.ReadFile(outFileFull); err != nil {
			return nil, fmt.Errorf("Can't read compiled source: %v", err)
		}
	}
	return &pb.CompileSourceReply{
		CompilerRetCode: int32(compilerProc.ProcessState.ExitCode()),
		CompiledSource:  compiledSource,
		CompilerStdout:  compilerStdout.Bytes(),
		CompilerStderr:  compilerStderr.Bytes(),
	}, nil
}

// CloseSession ...
func (s *CompilationServer) CloseSession(ctx context.Context, in *pb.CloseSessionRequest) (*pb.CloseSessionReply, error) {
	session := s.Sessions.GetSession(in.SessionID)
	if session == nil {
		return nil, fmt.Errorf("Unknown SessionID %d", in.SessionID)
	}
	s.closeSession(session, in.SessionID, true)
	return &pb.CloseSessionReply{}, nil
}

// Status ...
func (s *CompilationServer) Status(ctx context.Context, in *pb.StatusRequest) (*pb.StatusReply, error) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	cachedHeaderOnDiskCount, cachedHeaderOnDiskBytes := s.HeaderFileCache.GetFilesCountAndDiskUsage()
	return &pb.StatusReply{
		ServerVersion:           common.GetVersion(),
		CPUsCount:               uint64(runtime.NumCPU()),
		ActiveGoroutinesCount:   uint64(runtime.NumGoroutine()),
		ClientsCount:            s.UserCache.GetCachesCount(),
		SystemHeadersUsedCount:  s.SystemHeaders.GetSystemHeadersCacheSize(),
		CachedHeaderOnDiskCount: uint64(cachedHeaderOnDiskCount),
		CachedHeaderOnDiskBytes: uint64(cachedHeaderOnDiskBytes),
		HeapAllocBytes:          m.HeapAlloc,
		SystemAllocBytes:        m.Sys,
		UptimeNanoseconds:       uint64(time.Since(s.StartTime).Nanoseconds()),
	}, nil
}

func waitAndStop(grpcServer *grpc.Server) {
	time.Sleep(time.Millisecond * 100)
	grpcServer.GracefulStop()
}

func (s *CompilationServer) checkPassword(password string) error {
	if len(s.RemoteControlPassword) == 0 {
		return fmt.Errorf("Remote control disabled")
	}
	if s.RemoteControlPassword != password {
		s.RemoteControlPassword = ""
		return fmt.Errorf("Invalid password, disabling remote control")
	}
	return nil
}

// UpdateServer ...
func (s *CompilationServer) UpdateServer(ctx context.Context, in *pb.UpdateServerRequest) (*pb.UpdateServerReply, error) {
	s.remoteControlLock.Lock()
	defer s.remoteControlLock.Unlock()
	if err := s.checkPassword(in.Password); err != nil {
		return nil, err
	}

	newServerBinaryPath := path.Join(s.WorkingDir, "new-popcorn-server")
	if err := ioutil.WriteFile(newServerBinaryPath, in.NewBinary, 0777); err != nil {
		return nil, fmt.Errorf("Can't write new server binary: %v", err)
	}
	s.NewPopcornServerBinaryPath = newServerBinaryPath
	go waitAndStop(s.GRPCServer)
	return &pb.UpdateServerReply{}, nil
}

// RestartServer ...
func (s *CompilationServer) RestartServer(ctx context.Context, in *pb.RestartServerRequest) (*pb.RestartServerReply, error) {
	s.remoteControlLock.Lock()
	defer s.remoteControlLock.Unlock()
	if err := s.checkPassword(in.Password); err != nil {
		return nil, err
	}

	serverBinaryPath, err := filepath.EvalSymlinks("/proc/self/exe")
	if err != nil {
		return nil, fmt.Errorf("Can't get popcorn-server bin path: %v", err)
	}
	s.NewPopcornServerBinaryPath = serverBinaryPath
	go waitAndStop(s.GRPCServer)
	return &pb.RestartServerReply{}, nil
}

// DumpServerLog ...
func (s *CompilationServer) DumpServerLog(ctx context.Context, in *pb.DumpServerLogRequest) (*pb.DumpServerLogReply, error) {
	s.remoteControlLock.Lock()
	defer s.remoteControlLock.Unlock()
	if err := s.checkPassword(in.Password); err != nil {
		return nil, err
	}

	logFilename := common.GetLogFileName()
	if len(logFilename) == 0 {
		return nil, fmt.Errorf("Server is working without log file")
	}

	if in.BytesLimit == 0 {
		logData, err := ioutil.ReadFile(logFilename)
		if err != nil {
			return nil, fmt.Errorf("Can't read popcorn-server log file: %v", err)
		}
		return &pb.DumpServerLogReply{LogData: logData}, nil
	}

	f, err := os.Open(logFilename)
	if err != nil {
		return nil, fmt.Errorf("Can't open popcorn-server log file: %v", err)
	}
	defer f.Close()

	logBuffer := bytes.Buffer{}
	if _, err := io.CopyN(&logBuffer, f, int64(in.BytesLimit)); err != nil {
		return nil, fmt.Errorf("Can't read data from popcorn-server log file: %v", err)
	}
	return &pb.DumpServerLogReply{LogData: logBuffer.Bytes()}, nil
}
