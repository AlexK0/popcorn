package server

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"strings"
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

	GRPCServer *grpc.Server

	UserCaches       *UserCaches
	UploadingHeaders *SendingHeaders
	SystemHeaders    *SystemHeaderCache
	HeaderFileCache  *FileCache

	Sessions *UserSessions

	Stats *CompilationServerStats
}

// StartCompilationSession ...
func (s *CompilationServer) StartCompilationSession(ctx context.Context, in *pb.StartCompilationSessionRequest) (*pb.StartCompilationSessionReply, error) {
	callObserver := s.Stats.StartCompilationSession.StartRPCCall()

	userID := common.SHA256MessageToSHA256Struct(in.UserID)
	session := &UserSession{
		UserID:          userID,
		Compiler:        in.Compiler,
		CompilerArgs:    in.CompilerArgs,
		RequiredHeaders: make([]RequiredHeaderMetadata, 0, len(in.RequiredHeaders)),
		SourceFilePath:  in.SourceFilePath,
		FileSHA256Cache: s.UserCaches.GetFilesCache(userID),
	}

	sessionID := s.Sessions.OpenNewSession(session)
	session.WorkingDir = path.Join(s.SessionsDir, fmt.Sprint(sessionID))

	if err := os.MkdirAll(session.WorkingDir, os.ModePerm); err != nil {
		s.Sessions.CloseSession(sessionID)
		callObserver.FinishWithError()
		return nil, fmt.Errorf("Can't create session working directory: %v", err)
	}

	defer callObserver.Finish()
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
	callObserver := s.Stats.SendHeaderSHA256.StartRPCCall()

	session := s.Sessions.GetSession(in.SessionID)
	if session == nil {
		callObserver.FinishWithError()
		return nil, fmt.Errorf("Unknown SessionID %d", in.SessionID)
	}

	defer callObserver.Finish()
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
	callObserver := s.Stats.SendHeader.StartRPCCall()

	session := s.Sessions.GetSession(in.SessionID)
	if session == nil {
		callObserver.FinishWithError()
		return nil, fmt.Errorf("Unknown SessionID %d", in.SessionID)
	}

	headerMetadata := &session.RequiredHeaders[in.HeaderIndex]
	defer s.UploadingHeaders.FinishHeaderSending(headerMetadata.FilePath, headerMetadata.SHA256Struct)
	headerPathInWorkingDir := path.Join(session.WorkingDir, headerMetadata.FilePath)
	if s.HeaderFileCache.CreateLinkFromCache(headerMetadata.FilePath, headerMetadata.SHA256Struct, headerPathInWorkingDir) {
		s.Stats.SendingHeadersDoubleReceived.Increment()
		callObserver.Finish()
		return &pb.SendHeaderReply{}, nil
	}

	if err := common.WriteFile(headerPathInWorkingDir, in.HeaderBody); err != nil {
		callObserver.FinishWithError()
		return nil, fmt.Errorf("Can't save header: %v", err)
	}

	s.Stats.SendingHeadersReceived.Increment()
	_, _ = s.HeaderFileCache.SaveFileToCache(headerPathInWorkingDir, headerMetadata.FilePath, headerMetadata.SHA256Struct, int64(len(in.HeaderBody)))
	callObserver.Finish()
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
	callObserver := s.Stats.CompileSource.StartRPCCall()

	session := s.Sessions.GetSession(in.SessionID)
	if session == nil {
		callObserver.FinishWithError()
		return nil, fmt.Errorf("Unknown SessionID %d", in.SessionID)
	}

	defer s.closeSession(session, in.SessionID, in.CloseSessionAfterBuild)

	inFileFull := path.Join(session.WorkingDir, session.SourceFilePath)
	err := common.WriteFile(inFileFull, in.SourceBody)
	if err != nil {
		callObserver.FinishWithError()
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
			callObserver.FinishWithError()
			return nil, fmt.Errorf("Can't read compiled source: %v", err)
		}
	}
	callObserver.Finish()
	return &pb.CompileSourceReply{
		CompilerRetCode: int32(compilerProc.ProcessState.ExitCode()),
		CompiledSource:  compiledSource,
		CompilerStdout:  compilerStdout.Bytes(),
		CompilerStderr:  compilerStderr.Bytes(),
	}, nil
}

// CloseSession ...
func (s *CompilationServer) CloseSession(ctx context.Context, in *pb.CloseSessionRequest) (*pb.CloseSessionReply, error) {
	callObserver := s.Stats.CloseSession.StartRPCCall()
	session := s.Sessions.GetSession(in.SessionID)
	if session == nil {
		callObserver.FinishWithError()
		return nil, fmt.Errorf("Unknown SessionID %d", in.SessionID)
	}
	s.closeSession(session, in.SessionID, true)
	callObserver.Finish()
	return &pb.CloseSessionReply{}, nil
}

// Status ...
func (s *CompilationServer) Status(ctx context.Context, in *pb.StatusRequest) (*pb.StatusReply, error) {
	return &pb.StatusReply{
		ServerVersion: common.GetVersion(),
		ServerArgs:    os.Args,
		ServerStats:   s.Stats.GetStatsRawBytes(s),
	}, nil
}
