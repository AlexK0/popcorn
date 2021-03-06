package server

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
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

func (s *CompilationServer) startCompilationIfPossible(session *ClientSession, dependencies int) {
	if atomic.AddInt32(&session.CompilationStartDependencies, int32(dependencies)) == 0 {
		session.CompilationWaitFinish.Add(1)
		go s.performCompilation(session)
	}
}

func (s *CompilationServer) StartCompilationSession(ctx context.Context, in *pb.StartCompilationSessionRequest) (*pb.StartCompilationSessionReply, error) {
	callObserver := s.Stats.StartCompilationSession.StartRPCCall()
	sessionID, session := s.ActiveSessions.OpenNewSession(in, s.SessionsDir, s.RemoteClients.GetClient(common.SHA256MessageToSHA256Struct(in.ClientID)))

	if err := os.MkdirAll(session.WorkingDir, os.ModePerm); err != nil {
		s.ActiveSessions.CloseSession(sessionID)
		return nil, callObserver.FinishWithError(fmt.Errorf("Can't create session working directory: %v", err))
	}

	requiredFiles := make([]*pb.RequiredFile, 0, len(in.RequiredFiles))
	for index, fileMetadata := range session.RequiredFilesMeta {
		if fileMetadata.SHA256Struct.IsEmpty() {
			requiredFiles = append(requiredFiles, &pb.RequiredFile{FileIndex: uint32(index), Status: pb.RequiredStatus_SHA256_REQUIRED})
			continue
		}
		if s.SystemHeaders.IsSystemHeader(fileMetadata.FilePath, fileMetadata.FileSize, fileMetadata.SHA256Struct) {
			continue
		}
		if s.SrcFileCache.CreateLinkFromCache(fileMetadata.AbsPathInWorkingDir, fileMetadata.SHA256Struct) {
			continue
		}
		requiredFiles = append(requiredFiles, &pb.RequiredFile{FileIndex: uint32(index), Status: pb.RequiredStatus_FULL_COPY_REQUIRED})
	}

	s.startCompilationIfPossible(session, len(requiredFiles))
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
			s.startCompilationIfPossible(session, -1)
			_ = stream.Send(&pb.TransferFileReply{Status: pb.RequiredStatus_DONE})
			return callObserver.Finish()
		}
	} else if fileMetadata.SHA256Struct.IsEmpty() {
		return callObserver.FinishWithError(fmt.Errorf("SHA256 is required for %q", fileMetadata.FilePath))
	}

	start := time.Now()
	for {
		if s.SrcFileCache.CreateLinkFromCache(fileMetadata.AbsPathInWorkingDir, fileMetadata.SHA256Struct) {
			s.startCompilationIfPossible(session, -1)
			_ = stream.Send(&pb.TransferFileReply{Status: pb.RequiredStatus_DONE})
			return callObserver.Finish()
		}
		if s.UploadingFiles.StartFileTransfer(fileMetadata.FilePath, fileMetadata.SHA256Struct) {
			_ = stream.Send(&pb.TransferFileReply{Status: pb.RequiredStatus_FULL_COPY_REQUIRED})
			break
		}
		// TODO Why 6 seconds?
		if time.Since(start) > 6*time.Second {
			s.UploadingFiles.ForceStartFileTransfer(fileMetadata.FilePath, fileMetadata.SHA256Struct)
			_ = stream.Send(&pb.TransferFileReply{Status: pb.RequiredStatus_FULL_COPY_REQUIRED})
			s.Stats.ForceFileTransferring.Increment()
			break
		}
		// TODO Why 100 milliseconds?
		time.Sleep(100 * time.Millisecond)
	}

	defer s.UploadingFiles.FinishFileTransfer(fileMetadata.FilePath, fileMetadata.SHA256Struct)
	fileTmp, err := common.OpenTempFile(fileMetadata.AbsPathInWorkingDir)
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
	if err = os.Rename(fileTmp.Name(), fileMetadata.AbsPathInWorkingDir); err != nil {
		return clearTmpAndFinish(fmt.Errorf("Can't rename temp file: %v", err))
	}

	s.startCompilationIfPossible(session, -1)
	_ = stream.Send(&pb.TransferFileReply{Status: pb.RequiredStatus_DONE})
	_, _ = s.SrcFileCache.SaveFileToCache(fileMetadata.AbsPathInWorkingDir, fileMetadata.SHA256Struct, fileMetadata.FileSize)

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

func (s *CompilationServer) performCompilation(session *ClientSession) {
	objSHA256 := common.SHA256Struct{}
	objExtraKey := ""
	if session.UseObjectCache {
		objSHA256, objExtraKey = session.MakeObjectCacheKey()
		if s.ObjFileCache.CreateLinkFromCacheExtra(session.OutObjectFilePath, objSHA256, objExtraKey) {
			common.LogInfo("Get obj from cache", session.OutObjectFilePath)
			session.CompilationWaitFinish.Done()
			return
		}
	}

	compilerProc := exec.Command(session.Compiler, session.RemoveUnusedIncludeDirsAndGetCompilerArgs()...)
	compilerProc.Dir = session.WorkingDir
	var compilerStderrBuff, compilerStdoutBuff bytes.Buffer
	compilerProc.Stderr = &compilerStderrBuff
	compilerProc.Stdout = &compilerStdoutBuff

	common.LogInfo("Launch compiler:", compilerProc.Args)
	_ = compilerProc.Run()

	session.CompilerExitCode = compilerProc.ProcessState.ExitCode()
	session.CompilerStdout = compilerStdoutBuff.Bytes()
	session.CompilerStderr = compilerStderrBuff.Bytes()

	if session.CompilerExitCode == 0 && len(session.CompilerStdout) == 0 && len(session.CompilerStderr) == 0 && session.UseObjectCache {
		if stat, err := os.Stat(session.OutObjectFilePath); err == nil {
			_, _ = s.ObjFileCache.SaveFileToCacheExtra(session.OutObjectFilePath, objSHA256, objExtraKey, stat.Size())
		}
	}

	session.CompilationWaitFinish.Done()
}

func (s *CompilationServer) CompileSource(in *pb.CompileSourceRequest, stream pb.CompilationService_CompileSourceServer) error {
	callObserver := s.Stats.CompileSource.StartRPCCall()

	session := s.ActiveSessions.GetSession(in.SessionID)
	if session == nil {
		return callObserver.FinishWithError(fmt.Errorf("Unknown session %d", in.SessionID))
	}

	defer s.closeSession(session, in.SessionID, in.CloseSessionAfterBuild)

	if waitingFiles := atomic.LoadInt32(&session.CompilationStartDependencies); waitingFiles != 0 {
		return callObserver.FinishWithError(fmt.Errorf("Session %d is waiting %d files", in.SessionID, waitingFiles))
	}

	session.CompilationWaitFinish.Wait()
	if session.CompilerExitCode == 0 {
		if err := common.TransferFileByChunks(session.OutObjectFilePath, func(chunk []byte) error {
			if len(chunk) != 0 {
				return stream.Send(&pb.CompileSourceReply{
					Chunk: &pb.CompileSourceReply_CompiledObjChunk{
						CompiledObjChunk: chunk,
					},
				})
			}
			return nil
		}); err != nil {
			return callObserver.FinishWithError(fmt.Errorf("Can't send compiled source: %v", err))
		}
	}

	_ = stream.Send(&pb.CompileSourceReply{
		Chunk: &pb.CompileSourceReply_Epilogue{
			Epilogue: &pb.CompileSourceReply_StreamEpilogue{
				CompilerRetCode: int32(session.CompilerExitCode),
				CompilerStdout:  session.CompilerStdout,
				CompilerStderr:  session.CompilerStderr,
			},
		}})
	return callObserver.Finish()
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
	rawOut, _ := exec.Command(in.CheckCompiler, "-v").CombinedOutput()

	versionLine := "unknown"
	for _, line := range strings.Split(string(rawOut), "\n") {
		line = strings.TrimSpace(line)
		if strings.Contains(line, " version ") {
			versionLine = line
			break
		}
	}

	return &pb.StatusReply{
		ServerVersion:   common.GetVersion(),
		ServerArgs:      os.Args,
		ServerUptime:    int64(time.Since(s.StartTime)),
		CompilerVersion: versionLine,
	}, nil
}
