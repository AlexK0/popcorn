package client

import (
	"fmt"
	"os"

	pb "github.com/AlexK0/popcorn/internal/api/proto/v1"
	"github.com/AlexK0/popcorn/internal/common"
)

type RemoteCompiler struct {
	name          string
	inFile        string
	outFile       string
	remoteCmdArgs []string

	grpcClient     *GRPCClient
	clientID       *pb.SHA256Message
	clientUserName string
	sessionID      uint64

	needCloseSession bool
}

func MakeRemoteCompiler(localCompiler *LocalCompiler, serverHostPort string) (*RemoteCompiler, error) {
	clientUserName, clientID, err := common.MakeUniqueClientID()
	if err != nil {
		return nil, err
	}

	grpcClient, err := MakeGRPCClient(serverHostPort)
	if err != nil {
		return nil, err
	}

	return &RemoteCompiler{
		name:          localCompiler.name,
		inFile:        localCompiler.inFile,
		outFile:       localCompiler.outFile,
		remoteCmdArgs: localCompiler.MakeRemoteCmd(),

		grpcClient:     grpcClient,
		clientID:       clientID,
		clientUserName: clientUserName,
	}, nil
}

func (compiler *RemoteCompiler) transferFile(path string, index uint32, sha256Required bool, wg *common.WaitGroupWithError) {
	var fileSHA256Message *pb.SHA256Message = nil
	if sha256Required {
		fileSHA256, err := common.GetFileSHA256(path)
		if err != nil {
			wg.Done(fmt.Errorf("Can't calculate SHA256 for file %q: %v", path, err))
			return
		}
		fileSHA256Message = common.SHA256StructToSHA256Message(fileSHA256)
	}

	stream, err := compiler.grpcClient.Client.TransferFile(compiler.grpcClient.CallContext)
	if err != nil {
		wg.Done(fmt.Errorf("Can't open grpc stream: %v", err))
		return
	}

	err = stream.Send(&pb.TransferFileRequest{
		Chunk: &pb.TransferFileRequest_Header{
			Header: &pb.TransferFileRequest_StreamHeader{
				SessionID:  compiler.sessionID,
				FileIndex:  index,
				FileSHA256: fileSHA256Message,
			},
		},
	})
	if err != nil {
		wg.Done(fmt.Errorf("Can't send file metadata: %v", err))
		return
	}

	reply, err := stream.Recv()
	if err != nil {
		wg.Done(fmt.Errorf("Can't get transfer reply: %v", err))
		return
	}

	if reply.Status == pb.RequiredStatus_FULL_COPY_REQUIRED {
		if err = common.TransferFileByChunks(path, func(chunk []byte) error {
			return stream.Send(&pb.TransferFileRequest{Chunk: &pb.TransferFileRequest_FileBodyChunk{FileBodyChunk: chunk}})
		}); err != nil {
			wg.Done(err)
			return
		}

		reply, err = stream.Recv()
		if err != nil {
			wg.Done(fmt.Errorf("Can't get transfer finalization reply: %v", err))
			return
		}

		if reply.Status != pb.RequiredStatus_DONE {
			wg.Done(fmt.Errorf("Can't finalize file transferring: got unexpected status %v", reply.Status))
			return
		}
	}

	if err = stream.CloseSend(); err != nil {
		wg.Done(fmt.Errorf("Error on file transfering: %v", err))
	} else {
		wg.Done(nil)
	}
}

func (compiler *RemoteCompiler) SetupEnvironment(files []*pb.FileMetadata, useObjCache bool) error {
	clientCacheStream, err := compiler.grpcClient.Client.StartCompilationSession(
		compiler.grpcClient.CallContext,
		&pb.StartCompilationSessionRequest{
			ClientID:       compiler.clientID,
			ClientUserName: compiler.clientUserName,
			SourceFilePath: compiler.inFile,
			Compiler:       compiler.name,
			CompilerArgs:   compiler.remoteCmdArgs,
			RequiredFiles:  files,
			UseObjectCache: useObjCache,
		})
	if err != nil {
		return err
	}

	compiler.sessionID = clientCacheStream.SessionID
	compiler.needCloseSession = true

	sem := make(chan int, 6)
	wg := common.WaitGroupWithError{}
	wg.Add(len(clientCacheStream.RequiredFiles))
	for _, requiredFile := range clientCacheStream.RequiredFiles {
		sem <- 1
		go func(index uint32, sendSHA256 bool) {
			compiler.transferFile(files[index].FilePath, index, sendSHA256, &wg)
			<-sem
		}(requiredFile.FileIndex, requiredFile.Status == pb.RequiredStatus_SHA256_REQUIRED)
	}
	return wg.Wait()
}

func (compiler *RemoteCompiler) CompileSource() (retCode int, stdout []byte, stderr []byte, err error) {
	res, err := compiler.grpcClient.Client.CompileSource(
		compiler.grpcClient.CallContext,
		&pb.CompileSourceRequest{
			SessionID:              compiler.sessionID,
			CloseSessionAfterBuild: true,
		})

	if err != nil {
		return 0, nil, nil, err
	}

	defer func() { _ = res.CloseSend() }()
	compiler.needCloseSession = false

	chunk, err := res.Recv()
	if err != nil {
		return 0, nil, nil, fmt.Errorf("Can't receive first chunk for compiled obj %q: %v", compiler.outFile, err)
	}
	epilogue := chunk.GetEpilogue()
	if epilogue == nil {
		tmpFile, err := common.OpenTempFile(compiler.outFile)
		if err != nil {
			return 0, nil, nil, fmt.Errorf("Can't create temp file for saving %q: %v", compiler.outFile, err)
		}

		clearTmp := func(err error) (int, []byte, []byte, error) {
			tmpFile.Close()
			os.Remove(tmpFile.Name())
			return 0, nil, nil, err
		}

		for {
			if err != nil {
				return clearTmp(fmt.Errorf("Can't receive compiled obj %q: %v", compiler.outFile, err))
			}
			if fileChunk := chunk.GetCompiledObjChunk(); len(fileChunk) != 0 {
				if _, err = tmpFile.Write(fileChunk); err != nil {
					return clearTmp(fmt.Errorf("Can't save chunk of compiled obj %q: %v", compiler.outFile, err))
				}
				chunk, err = res.Recv()
			} else {
				epilogue = chunk.GetEpilogue()
				break
			}
		}

		tmpFile.Close()
		if err = os.Rename(tmpFile.Name(), compiler.outFile); err != nil {
			return clearTmp(fmt.Errorf("Can't rename compiled obj %q: %v", compiler.outFile, err))
		}
	}

	if epilogue == nil {
		return 0, nil, nil, fmt.Errorf("Epilogue for %q is missed", compiler.outFile)
	}

	return int(epilogue.CompilerRetCode), epilogue.CompilerStderr, epilogue.CompilerStdout, nil
}

func (compiler *RemoteCompiler) Clear() {
	if compiler.needCloseSession {
		_, _ = compiler.grpcClient.Client.CloseSession(
			compiler.grpcClient.CallContext,
			&pb.CloseSessionRequest{
				SessionID: compiler.sessionID,
			})
	}
	compiler.needCloseSession = false
	compiler.grpcClient.Clear()
}
