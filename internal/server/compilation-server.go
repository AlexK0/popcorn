package server

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
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

	GRPCServer *grpc.Server

	Clients          *Clients
	UploadingHeaders *SendingHeaders
	SystemHeaders    *SystemHeaderCache
	HeaderFileCache  *FileCache

	UserSessions *Sessions

	Stats *CompilationServerStats
}

// StartCompilationSession ...
func (s *CompilationServer) StartCompilationSession(ctx context.Context, in *pb.StartCompilationSessionRequest) (*pb.StartCompilationSessionReply, error) {
	callObserver := s.Stats.StartCompilationSession.StartRPCCall()
	sessionID, session := s.UserSessions.OpenNewSession(in, s.SessionsDir, s.Clients.GetUser(common.SHA256MessageToSHA256Struct(in.UserID)))

	if err := os.MkdirAll(session.WorkingDir, os.ModePerm); err != nil {
		s.UserSessions.CloseSession(sessionID)
		return nil, callObserver.FinishWithError(fmt.Errorf("Can't create session working directory: %v", err))
	}

	requiredFiles := make([]*pb.RequiredFile, 0, 1+len(in.RequiredHeaders))
	for index, requiredHeader := range session.RequiredHeaders {
		if requiredHeader.SHA256Struct.IsEmpty() {
			requiredFiles = append(requiredFiles, &pb.RequiredFile{HeaderIndex: int32(index), Status: pb.RequiredStatus_SHA256_REQUIRED})
			continue
		}
		if s.SystemHeaders.IsSystemHeader(requiredHeader.FilePath, requiredHeader.FileSize, requiredHeader.SHA256Struct) {
			requiredHeader.UseFromSystem = true
			continue
		}
		_, headerPathInWorkingDir := session.GetFilePathInWorkingDir(requiredHeader.FilePath)
		if s.HeaderFileCache.CreateLinkFromCache(requiredHeader.FilePath, requiredHeader.SHA256Struct, headerPathInWorkingDir) {
			continue
		}
		requiredFiles = append(requiredFiles, &pb.RequiredFile{HeaderIndex: int32(index), Status: pb.RequiredStatus_FULL_COPY_REQUIRED})
	}

	return &pb.StartCompilationSessionReply{
		SessionID:     sessionID,
		RequiredFiles: requiredFiles,
	}, callObserver.Finish()
}

func saveFileFromStream(file *os.File, stream pb.CompilationService_TransferFileServer) (int64, error) {
	fileSize := int64(0)
	for {
		request, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return 0, fmt.Errorf("Unexpected error on receiving header chunk: %v", err)
		}
		fileChunk := request.GetFileBodyChunk()
		if fileChunk == nil {
			return 0, fmt.Errorf("Header body chunk is expected")
		}
		if _, err = file.Write(fileChunk); err != nil {
			return 0, fmt.Errorf("Can't write header chunk: %v", err)
		}
		fileSize += int64(len(fileChunk))
	}
	return fileSize, nil
}

// TransferFile ...
func (s *CompilationServer) TransferFile(stream pb.CompilationService_TransferFileServer) error {
	callObserver := s.Stats.TransferFile.StartRPCCall()
	request, err := stream.Recv()
	if err != nil {
		return callObserver.FinishWithError(fmt.Errorf("Unexpected error: %v", err))
	}

	metadata := request.GetHeader()
	if metadata == nil {
		return callObserver.FinishWithError(fmt.Errorf("Metadata af first chunk is expected"))
	}

	session := s.UserSessions.GetSession(metadata.SessionID)
	if session == nil {
		return callObserver.FinishWithError(fmt.Errorf("Unknown SessionID %d", metadata.SessionID))
	}

	fileMetadata := &session.RequiredHeaders[metadata.FileIndex]
	if metadata.FileSHA256 != nil {
		fileMetadata.SHA256Struct = common.SHA256MessageToSHA256Struct(metadata.FileSHA256)
		session.UserInfo.HeaderSHA256Cache.SetFileSHA256(fileMetadata.FilePath, fileMetadata.MTime, fileMetadata.FileSize, fileMetadata.SHA256Struct)
		if s.SystemHeaders.IsSystemHeader(fileMetadata.FilePath, fileMetadata.FileSize, fileMetadata.SHA256Struct) {
			fileMetadata.UseFromSystem = true
			_ = stream.Send(&pb.TransferFileOut{FullCopyRequired: false})
			return callObserver.Finish()
		}
	} else if fileMetadata.SHA256Struct.IsEmpty() {
		return callObserver.FinishWithError(fmt.Errorf("SHA256 is required for %q", fileMetadata.FilePath))
	}

	_, headerPathInWorkingDir := session.GetFilePathInWorkingDir(fileMetadata.FilePath)
	start := time.Now()
	for {
		if s.HeaderFileCache.CreateLinkFromCache(fileMetadata.FilePath, fileMetadata.SHA256Struct, headerPathInWorkingDir) {
			_ = stream.Send(&pb.TransferFileOut{FullCopyRequired: false})
			return callObserver.Finish()
		}
		if s.UploadingHeaders.StartHeaderSending(fileMetadata.FilePath, fileMetadata.SHA256Struct) {
			_ = stream.Send(&pb.TransferFileOut{FullCopyRequired: true})
			break
		}
		// TODO Why 6 seconds?
		if time.Since(start) > 6*time.Second {
			s.UploadingHeaders.ForceStartHeaderSending(fileMetadata.FilePath, fileMetadata.SHA256Struct)
			_ = stream.Send(&pb.TransferFileOut{FullCopyRequired: true})
			break
		}
		// TODO Why 100 milliseconds?
		time.Sleep(100 * time.Millisecond)
	}

	defer s.UploadingHeaders.FinishHeaderSending(fileMetadata.FilePath, fileMetadata.SHA256Struct)
	headerFileTmp, err := common.OpenTempFile(headerPathInWorkingDir)
	if err != nil {
		return callObserver.FinishWithError(fmt.Errorf("Can't open temp file for saving header: %v", err))
	}

	transferredBytes, err := saveFileFromStream(headerFileTmp, stream)
	headerFileTmp.Close()
	clearTmpAndFinish := func(err error) error {
		os.Remove(headerFileTmp.Name())
		return callObserver.FinishWithError(err)
	}
	if err != nil {
		return clearTmpAndFinish(err)
	}
	if fileMetadata.FileSize != transferredBytes {
		return clearTmpAndFinish(fmt.Errorf("Mismatch transferred bytes count: received %d, expected %d", transferredBytes, fileMetadata.FileSize))
	}
	if err = os.Rename(headerFileTmp.Name(), headerPathInWorkingDir); err != nil {
		return clearTmpAndFinish(fmt.Errorf("Can't rename header temp file: %v", err))
	}

	common.LogInfo("File", fileMetadata.FilePath, "successfully transferred")
	s.Stats.SendingHeadersReceived.Increment()
	_, _ = s.HeaderFileCache.SaveFileToCache(headerPathInWorkingDir, fileMetadata.FilePath, fileMetadata.SHA256Struct, fileMetadata.FileSize)
	return callObserver.Finish()
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
		return nil, callObserver.FinishWithError(fmt.Errorf("Unknown SessionID %d", in.SessionID))
	}

	defer s.closeSession(session, in.SessionID, in.CloseSessionAfterBuild)

	err := common.WriteFile(session.SourceFilePath, in.SourceBody)
	if err != nil {
		return nil, callObserver.FinishWithError(fmt.Errorf("Can't write source for compilation: %v", err))
	}

	compilerProc := exec.Command(session.Compiler, session.RemoveUnusedIncludeDirsAndGetCompilerArgs()...)
	compilerProc.Dir = session.WorkingDir
	var compilerStderr, compilerStdout bytes.Buffer
	compilerProc.Stderr = &compilerStderr
	compilerProc.Stdout = &compilerStdout

	common.LogInfo("Launch compiler:", compilerProc.Args)
	_ = compilerProc.Run()

	var compiledSource []byte
	if compilerProc.ProcessState.ExitCode() == 0 {
		if compiledSource, err = ioutil.ReadFile(session.OutObjectFilePath); err != nil {
			return nil, callObserver.FinishWithError(fmt.Errorf("Can't read compiled source: %v", err))
		}
	}

	return &pb.CompileSourceReply{
		CompilerRetCode: int32(compilerProc.ProcessState.ExitCode()),
		CompiledSource:  compiledSource,
		CompilerStdout:  compilerStdout.Bytes(),
		CompilerStderr:  compilerStderr.Bytes(),
	}, callObserver.Finish()
}

// CloseSession ...
func (s *CompilationServer) CloseSession(ctx context.Context, in *pb.CloseSessionRequest) (*pb.CloseSessionReply, error) {
	callObserver := s.Stats.CloseSession.StartRPCCall()
	session := s.UserSessions.GetSession(in.SessionID)
	if session == nil {
		return nil, callObserver.FinishWithError(fmt.Errorf("Unknown SessionID %d", in.SessionID))
	}
	s.closeSession(session, in.SessionID, true)
	return &pb.CloseSessionReply{}, callObserver.Finish()
}

// Status ...
func (s *CompilationServer) Status(ctx context.Context, in *pb.StatusRequest) (*pb.StatusReply, error) {
	return &pb.StatusReply{
		ServerVersion: common.GetVersion(),
		ServerArgs:    os.Args,
		ServerStats:   s.Stats.GetStatsRawBytes(s),
	}, nil
}
