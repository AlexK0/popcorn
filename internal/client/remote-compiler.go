package client

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"

	pb "github.com/AlexK0/popcorn/internal/api/proto/v1"
	"github.com/AlexK0/popcorn/internal/common"
)

// RemoteCompiler ...
type RemoteCompiler struct {
	name          string
	inFile        string
	outFile       string
	remoteCmdArgs []string

	grpcClient *GRPCClient
	userID     *pb.SHA256Message
	userName   string
	sessionID  uint64

	needCloseSession bool
}

// MakeRemoteCompiler ...
func MakeRemoteCompiler(localCompiler *LocalCompiler, serverHostPort string) (*RemoteCompiler, error) {
	userName, userID, err := common.MakeUniqueUserID()
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

		grpcClient: grpcClient,
		userID:     userID,
		userName:   userName,
	}, nil
}

func transferFileByChunks(path string, stream pb.CompilationService_TransferFileClient) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("Can't open file %q for sending: %v", path, err)
	}
	defer file.Close()

	var buffer [256 * 1024]byte
	for {
		n, err := file.Read(buffer[:])
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("Can't read file %q: %v", path, err)
		}
		if err = stream.Send(&pb.TransferFileIn{Chunk: &pb.TransferFileIn_FileBodyChunk{FileBodyChunk: buffer[:n]}}); err != nil {
			return fmt.Errorf("Can't transfer file %q: %v", path, err)
		}
	}
}

func (compiler *RemoteCompiler) transferFile(path string, index int32, sha256Required bool, wg *common.WaitGroupWithError) {
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

	err = stream.Send(&pb.TransferFileIn{
		Chunk: &pb.TransferFileIn_Header{
			Header: &pb.TransferFileIn_StreamHeader{
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

	if reply.FullCopyRequired {
		if err = transferFileByChunks(path, stream); err != nil {
			wg.Done(err)
			return
		}
	}

	if err = stream.CloseSend(); err != nil {
		wg.Done(fmt.Errorf("Error on file transfering: %v", err))
	} else {
		wg.Done(nil)
	}
}

func (compiler *RemoteCompiler) SetupEnvironment(headers []*pb.FileMetadata) error {
	clientCacheStream, err := compiler.grpcClient.Client.StartCompilationSession(
		compiler.grpcClient.CallContext,
		&pb.StartCompilationSessionRequest{
			UserID:          compiler.userID,
			UserName:        compiler.userName,
			SourceFilePath:  compiler.inFile,
			Compiler:        compiler.name,
			CompilerArgs:    compiler.remoteCmdArgs,
			RequiredHeaders: headers,
		})
	if err != nil {
		return err
	}

	compiler.sessionID = clientCacheStream.SessionID
	compiler.needCloseSession = true

	sem := make(chan int, 8)
	wg := common.WaitGroupWithError{}
	wg.Add(len(clientCacheStream.MissedHeadersSHA256) + len(clientCacheStream.MissedHeadersFullCopy))
	for _, index := range clientCacheStream.MissedHeadersSHA256 {
		sem <- 1
		go func(index int32) {
			compiler.transferFile(headers[index].FilePath, index, true, &wg)
			<-sem
		}(index)
	}
	for _, index := range clientCacheStream.MissedHeadersFullCopy {
		sem <- 1
		go func(index int32) {
			compiler.transferFile(headers[index].FilePath, index, false, &wg)
			<-sem
		}(index)
	}
	return wg.Wait()
}

func (compiler *RemoteCompiler) CompileSource() (retCode int, stdout []byte, stderr []byte, err error) {
	sourceBody, err := ioutil.ReadFile(compiler.inFile)
	if err != nil {
		return 0, nil, nil, err
	}
	res, err := compiler.grpcClient.Client.CompileSource(
		compiler.grpcClient.CallContext,
		&pb.CompileSourceRequest{
			SessionID:              compiler.sessionID,
			SourceBody:             sourceBody,
			CloseSessionAfterBuild: true,
		})

	if err != nil {
		return 0, nil, nil, err
	}
	compiler.needCloseSession = false

	if res.CompilerRetCode == 0 {
		if err = common.WriteFile(compiler.outFile, res.CompiledSource); err != nil {
			return 0, nil, nil, err
		}
	}

	return int(res.CompilerRetCode), res.CompilerStderr, res.CompilerStdout, nil
}

// Clear ...
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
