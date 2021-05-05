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

	Users            *Users
	UploadingHeaders *SendingHeaders
	SystemHeaders    *SystemHeaderCache
	HeaderFileCache  *FileCache

	UserSessions *Sessions

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
		UserInfo:        s.Users.GetUser(userID),
	}

	sessionID := s.UserSessions.OpenNewSession(session)
	session.WorkingDir = path.Join(s.SessionsDir, fmt.Sprint(sessionID))

	if err := os.MkdirAll(session.WorkingDir, os.ModePerm); err != nil {
		s.UserSessions.CloseSession(sessionID)
		callObserver.FinishWithError()
		return nil, fmt.Errorf("Can't create session working directory: %v", err)
	}

	defer callObserver.Finish()
	missedHeadersSHA256 := make([]int32, 0, len(in.RequiredHeaders))
	missedHeadersFullCopy := make([]int32, 0, len(in.RequiredHeaders))
	for index, headerMetadata := range in.RequiredHeaders {
		headerSHA256, ok := session.UserInfo.HeaderSHA256Cache.GetFileSHA256(headerMetadata.FilePath, headerMetadata.MTime)
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
		SessionID:             s.UserSessions.OpenNewSession(session),
		MissedHeadersSHA256:   missedHeadersSHA256,
		MissedHeadersFullCopy: missedHeadersFullCopy,
	}, nil
}

// SendHeaderSHA256 ...
func (s *CompilationServer) SendHeaderSHA256(ctx context.Context, in *pb.SendHeaderSHA256Request) (*pb.SendHeaderSHA256Reply, error) {
	callObserver := s.Stats.SendHeaderSHA256.StartRPCCall()

	session := s.UserSessions.GetSession(in.SessionID)
	if session == nil {
		callObserver.FinishWithError()
		return nil, fmt.Errorf("Unknown SessionID %d", in.SessionID)
	}

	defer callObserver.Finish()
	headerMetadata := &session.RequiredHeaders[in.HeaderIndex]
	headerMetadata.SHA256Struct = common.SHA256MessageToSHA256Struct(in.HeaderSHA256)
	session.UserInfo.HeaderSHA256Cache.SetFileSHA256(headerMetadata.FilePath, headerMetadata.MTime, headerMetadata.SHA256Struct)
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

func saveFileFromStream(file *os.File, stream pb.CompilationService_SendHeaderServer) (int64, error) {
	headerFileSize := int64(0)
	for {
		request, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return 0, fmt.Errorf("Unexpected error on receiving header chunk: %v", err)
		}
		headerChunk := request.GetHeaderBodyChunk()
		if headerChunk == nil {
			return 0, fmt.Errorf("Header body chunk is expected")
		}
		if _, err = file.Write(headerChunk); err != nil {
			return 0, fmt.Errorf("Can't write header chunk: %v", err)
		}
		headerFileSize += int64(len(headerChunk))
	}
	return headerFileSize, nil
}

// SendHeader ...
func (s *CompilationServer) SendHeader(stream pb.CompilationService_SendHeaderServer) error {
	callObserver := s.Stats.SendHeader.StartRPCCall()

	request, err := stream.Recv()
	if err != nil {
		callObserver.FinishWithError()
		return fmt.Errorf("Unexpected error: %v", err)
	}

	defer stream.SendAndClose(&pb.SendHeaderReply{})
	metadata := request.GetMetadata()
	if metadata == nil {
		callObserver.FinishWithError()
		return fmt.Errorf("Metadata af first chunk is expected")
	}

	session := s.UserSessions.GetSession(metadata.SessionID)
	if session == nil {
		callObserver.FinishWithError()
		return fmt.Errorf("Unknown SessionID %d", metadata.SessionID)
	}

	headerMetadata := &session.RequiredHeaders[metadata.HeaderIndex]
	defer s.UploadingHeaders.FinishHeaderSending(headerMetadata.FilePath, headerMetadata.SHA256Struct)
	headerPathInWorkingDir := path.Join(session.WorkingDir, headerMetadata.FilePath)
	if s.HeaderFileCache.CreateLinkFromCache(headerMetadata.FilePath, headerMetadata.SHA256Struct, headerPathInWorkingDir) {
		s.Stats.SendingHeadersDoubleReceived.Increment()
		callObserver.Finish()
		return nil
	}

	headerFileTmp, err := common.OpenTempFile(headerPathInWorkingDir)
	if err != nil {
		callObserver.FinishWithError()
		return fmt.Errorf("Can't open temp file for saving header: %v", err)
	}

	headerFileSize, err := saveFileFromStream(headerFileTmp, stream)
	headerFileTmp.Close()
	if err != nil {
		os.Remove(headerFileTmp.Name())
		callObserver.FinishWithError()
		return err
	}

	if err = os.Rename(headerFileTmp.Name(), headerPathInWorkingDir); err != nil {
		os.Remove(headerFileTmp.Name())
		callObserver.FinishWithError()
		return fmt.Errorf("Can't rename header temp file: %v", err)
	}

	s.Stats.SendingHeadersReceived.Increment()
	_, _ = s.HeaderFileCache.SaveFileToCache(headerPathInWorkingDir, headerMetadata.FilePath, headerMetadata.SHA256Struct, headerFileSize)
	callObserver.Finish()
	return nil
}

func (s *CompilationServer) closeSession(session *UserSession, sessionID uint64, close bool) {
	if close {
		s.UserSessions.CloseSession(sessionID)
		_ = os.RemoveAll(session.WorkingDir)
	}
}

// CompileSource ....
func (s *CompilationServer) CompileSource(ctx context.Context, in *pb.CompileSourceRequest) (*pb.CompileSourceReply, error) {
	callObserver := s.Stats.CompileSource.StartRPCCall()

	session := s.UserSessions.GetSession(in.SessionID)
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
	session := s.UserSessions.GetSession(in.SessionID)
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
