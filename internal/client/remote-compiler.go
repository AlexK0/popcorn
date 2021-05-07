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

func (compiler *RemoteCompiler) readHeaderAndSendSHA256OrBody(path string, mtime int64, index int32, wg *common.WaitGroupWithError) {
	headerSha256, err := common.GetFileSHA256(path)
	if err != nil {
		wg.Done(err)
		return
	}
	reply, err := compiler.grpcClient.Client.SendFileSHA256(
		compiler.grpcClient.CallContext,
		&pb.SendFileSHA256Request{
			SessionID:    compiler.sessionID,
			HeaderIndex:  index,
			HeaderSHA256: common.SHA256StructToSHA256Message(headerSha256),
		})
	if err == nil && reply.FullCopyRequired {
		compiler.readHeaderAndSend(path, index, wg)
	} else {
		wg.Done(err)
	}
}

func (compiler *RemoteCompiler) readHeaderAndSend(path string, index int32, wg *common.WaitGroupWithError) {
	file, err := os.Open(path)
	if err != nil {
		wg.Done(fmt.Errorf("Can't open file %q for sending: %v", path, err))
		return
	}
	defer file.Close()

	stream, err := compiler.grpcClient.Client.TransferFile(compiler.grpcClient.CallContext)
	if err != nil {
		wg.Done(fmt.Errorf("Can't open grpc stream: %v", err))
		return
	}

	err = stream.Send(&pb.TransferFileStream{
		Chunk: &pb.TransferFileStream_Header{
			Header: &pb.TransferFileStream_StreamHeader{
				SessionID:   compiler.sessionID,
				HeaderIndex: index,
			},
		},
	})
	if err != nil {
		wg.Done(fmt.Errorf("Can't send file metadata: %v", err))
		return
	}

	var buffer [256 * 1024]byte
	for {
		n, err := file.Read(buffer[:])
		if err == io.EOF {
			break
		}
		if err != nil {
			wg.Done(fmt.Errorf("Can't read file %q: %v", path, err))
			return
		}
		err = stream.Send(&pb.TransferFileStream{
			Chunk: &pb.TransferFileStream_FileBodyChunk{
				FileBodyChunk: buffer[:n],
			},
		})
		if err == io.EOF {
			break
		}
	}

	if _, err = stream.CloseAndRecv(); err != nil {
		wg.Done(fmt.Errorf("Can't send file: %v", err))
	} else {
		wg.Done(nil)
	}
}

// SetupEnvironment ...
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
			compiler.readHeaderAndSendSHA256OrBody(headers[index].FilePath, headers[index].MTime, index, &wg)
			<-sem
		}(index)
	}
	for _, index := range clientCacheStream.MissedHeadersFullCopy {
		sem <- 1
		go func(index int32) {
			compiler.readHeaderAndSend(headers[index].FilePath, index, &wg)
			<-sem
		}(index)
	}
	return wg.Wait()
}

// CompileSource ...
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
