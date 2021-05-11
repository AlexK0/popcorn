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

type CompilationServer struct {
	pb.UnimplementedCompilationServiceServer

	StartTime time.Time

	SessionsDir string

	GRPCServer *grpc.Server

	RemoteClients  *Clients
	UploadingFiles *FileTransferring
	SystemHeaders  *SystemHeaderCache
	SrcFileCache   *FileCache
	ObjFileCache   *FileCache

	ActiveSessions *Sessions

	Stats *CompilationServerStats
}

func (s *CompilationServer) StartCompilationSession(ctx context.Context, in *pb.StartCompilationSessionRequest) (*pb.StartCompilationSessionReply, error) {
	callObserver := s.Stats.StartCompilationSession.StartRPCCall()
	sessionID, session := s.ActiveSessions.OpenNewSession(in, s.SessionsDir, s.RemoteClients.GetClient(common.SHA256MessageToSHA256Struct(in.ClientID)))

	if err := os.MkdirAll(session.WorkingDir, os.ModePerm); err != nil {
		s.ActiveSessions.CloseSession(sessionID)
		return nil, callObserver.FinishWithError(fmt.Errorf("Can't create session working directory: %v", err))
	}

	requiredFiles := make([]*pb.RequiredFile, 0, 1+len(in.RequiredFiles))
	for index, requiredFile := range session.RequiredFilesMeta {
		if requiredFile.SHA256Struct.IsEmpty() {
			requiredFiles = append(requiredFiles, &pb.RequiredFile{FileIndex: uint32(index), Status: pb.RequiredStatus_SHA256_REQUIRED})
			continue
		}
		if s.SystemHeaders.IsSystemHeader(requiredFile.FilePath, requiredFile.FileSize, requiredFile.SHA256Struct) {
			continue
		}
		_, filePathInWorkingDir := session.GetFilePathInWorkingDir(requiredFile.FilePath)
		if s.SrcFileCache.CreateLinkFromCache(requiredFile.FilePath, requiredFile.SHA256Struct, filePathInWorkingDir) {
			continue
		}
		requiredFiles = append(requiredFiles, &pb.RequiredFile{FileIndex: uint32(index), Status: pb.RequiredStatus_FULL_COPY_REQUIRED})
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
			return 0, fmt.Errorf("Unexpected error on receiving file chunk: %v", err)
		}
		fileChunk := request.GetFileBodyChunk()
		if fileChunk == nil {
			return 0, fmt.Errorf("File body chunk is expected")
		}
		if len(fileChunk) == 0 {
			break
		}
		if _, err = file.Write(fileChunk); err != nil {
			return 0, fmt.Errorf("Can't write file chunk: %v", err)
		}
		fileSize += int64(len(fileChunk))
	}
	return fileSize, nil
}

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

	session := s.ActiveSessions.GetSession(metadata.SessionID)
	if session == nil {
		return callObserver.FinishWithError(fmt.Errorf("Unknown SessionID %d", metadata.SessionID))
	}

	fileMetadata := &session.RequiredFilesMeta[metadata.FileIndex]
	if metadata.FileSHA256 != nil {
		fileMetadata.SHA256Struct = common.SHA256MessageToSHA256Struct(metadata.FileSHA256)
		session.ClientInfo.FileSHA256Cache.SetFileSHA256(fileMetadata.FilePath, fileMetadata.MTime, fileMetadata.FileSize, fileMetadata.SHA256Struct)
		if s.SystemHeaders.IsSystemHeader(fileMetadata.FilePath, fileMetadata.FileSize, fileMetadata.SHA256Struct) {
			_ = stream.Send(&pb.TransferFileOut{Status: pb.RequiredStatus_DONE})
			return callObserver.Finish()
		}
	} else if fileMetadata.SHA256Struct.IsEmpty() {
		return callObserver.FinishWithError(fmt.Errorf("SHA256 is required for %q", fileMetadata.FilePath))
	}

	_, filePathInWorkingDirAbs := session.GetFilePathInWorkingDir(fileMetadata.FilePath)
	start := time.Now()
	for {
		if s.SrcFileCache.CreateLinkFromCache(fileMetadata.FilePath, fileMetadata.SHA256Struct, filePathInWorkingDirAbs) {
			_ = stream.Send(&pb.TransferFileOut{Status: pb.RequiredStatus_DONE})
			return callObserver.Finish()
		}
		if s.UploadingFiles.StartFileTransfer(fileMetadata.FilePath, fileMetadata.SHA256Struct) {
			_ = stream.Send(&pb.TransferFileOut{Status: pb.RequiredStatus_FULL_COPY_REQUIRED})
			break
		}
		// TODO Why 6 seconds?
		if time.Since(start) > 6*time.Second {
			s.UploadingFiles.ForceStartFileTransfer(fileMetadata.FilePath, fileMetadata.SHA256Struct)
			_ = stream.Send(&pb.TransferFileOut{Status: pb.RequiredStatus_FULL_COPY_REQUIRED})
			break
		}
		// TODO Why 100 milliseconds?
		time.Sleep(100 * time.Millisecond)
	}

	defer s.UploadingFiles.FinishFileTransfer(fileMetadata.FilePath, fileMetadata.SHA256Struct)
	fileTmp, err := common.OpenTempFile(filePathInWorkingDirAbs)
	if err != nil {
		return callObserver.FinishWithError(fmt.Errorf("Can't open temp file for saving transferring file: %v", err))
	}

	transferredBytes, err := saveFileFromStream(fileTmp, stream)
	fileTmp.Close()
	clearTmpAndFinish := func(err error) error {
		os.Remove(fileTmp.Name())
		return callObserver.FinishWithError(err)
	}
	if err != nil {
		return clearTmpAndFinish(err)
	}
	if fileMetadata.FileSize != transferredBytes {
		return clearTmpAndFinish(fmt.Errorf("Mismatch transferred bytes count: received %d, expected %d", transferredBytes, fileMetadata.FileSize))
	}
	if err = os.Rename(fileTmp.Name(), filePathInWorkingDirAbs); err != nil {
		return clearTmpAndFinish(fmt.Errorf("Can't rename temp file: %v", err))
	}

	_ = stream.Send(&pb.TransferFileOut{Status: pb.RequiredStatus_DONE})
	_, _ = s.SrcFileCache.SaveFileToCache(filePathInWorkingDirAbs, fileMetadata.FilePath, fileMetadata.SHA256Struct, fileMetadata.FileSize)

	s.Stats.TransferredFiles.Increment()
	common.LogInfo("File", fileMetadata.FilePath, "successfully transferred")
	return callObserver.Finish()
}

func (s *CompilationServer) closeSession(session *ClientSession, sessionID uint64, close bool) {
	if close {
		s.ActiveSessions.CloseSession(sessionID)
		_ = os.RemoveAll(session.WorkingDir)
	}
}

func (s *CompilationServer) performCompilation(session *ClientSession) (exitCode int, compilerStdout []byte, compilerStderr []byte) {
	objSHA256 := common.SHA256Struct{}
	objExtraKey := ""
	if session.UseObjectCache {
		objSHA256, objExtraKey = session.MakeObjectCacheKey()
		if s.ObjFileCache.CreateLinkFromCacheExtra(session.OutObjectFilePath, objSHA256, objExtraKey, session.OutObjectFilePath) {
			common.LogInfo("Get obj from cache", session.OutObjectFilePath)
			return 0, nil, nil
		}
	}

	compilerProc := exec.Command(session.Compiler, session.RemoveUnusedIncludeDirsAndGetCompilerArgs()...)
	compilerProc.Dir = session.WorkingDir
	var compilerStderrBuff, compilerStdoutBuff bytes.Buffer
	compilerProc.Stderr = &compilerStderrBuff
	compilerProc.Stdout = &compilerStdoutBuff

	common.LogInfo("Launch compiler:", compilerProc.Args)
	_ = compilerProc.Run()

	exitCode = compilerProc.ProcessState.ExitCode()
	compilerStdout = compilerStdoutBuff.Bytes()
	compilerStderr = compilerStderrBuff.Bytes()

	if exitCode == 0 && len(compilerStdout) == 0 && len(compilerStderr) == 0 && session.UseObjectCache {
		if stat, err := os.Stat(session.OutObjectFilePath); err == nil {
			_, _ = s.ObjFileCache.SaveFileToCacheExtra(session.OutObjectFilePath, session.OutObjectFilePath, objSHA256, objExtraKey, stat.Size())
		}
	}

	return
}

func (s *CompilationServer) CompileSource(ctx context.Context, in *pb.CompileSourceRequest) (*pb.CompileSourceReply, error) {
	callObserver := s.Stats.CompileSource.StartRPCCall()

	session := s.ActiveSessions.GetSession(in.SessionID)
	if session == nil {
		return nil, callObserver.FinishWithError(fmt.Errorf("Unknown SessionID %d", in.SessionID))
	}

	defer s.closeSession(session, in.SessionID, in.CloseSessionAfterBuild)

	exitCode, compilerStdout, compilerStderr := s.performCompilation(session)
	var compiledSource []byte
	var err error
	if exitCode == 0 {
		if compiledSource, err = ioutil.ReadFile(session.OutObjectFilePath); err != nil {
			return nil, callObserver.FinishWithError(fmt.Errorf("Can't read compiled source: %v", err))
		}
	}

	return &pb.CompileSourceReply{
		CompilerRetCode: int32(exitCode),
		CompiledSource:  compiledSource,
		CompilerStdout:  compilerStdout,
		CompilerStderr:  compilerStderr,
	}, callObserver.Finish()
}

func (s *CompilationServer) CloseSession(ctx context.Context, in *pb.CloseSessionRequest) (*pb.CloseSessionReply, error) {
	callObserver := s.Stats.CloseSession.StartRPCCall()
	session := s.ActiveSessions.GetSession(in.SessionID)
	if session == nil {
		return nil, callObserver.FinishWithError(fmt.Errorf("Unknown SessionID %d", in.SessionID))
	}
	s.closeSession(session, in.SessionID, true)
	return &pb.CloseSessionReply{}, callObserver.Finish()
}

func (s *CompilationServer) Status(ctx context.Context, in *pb.StatusRequest) (*pb.StatusReply, error) {
	return &pb.StatusReply{
		ServerVersion: common.GetVersion(),
		ServerArgs:    os.Args,
		ServerStats:   s.Stats.GetStatsRawBytes(s),
	}, nil
}
