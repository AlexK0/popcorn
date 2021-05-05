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
	sessionID  uint64

	needCloseSession        bool
	headersSha256Calculator HeaderSHA256Calculator
}

// MakeRemoteCompiler ...
func MakeRemoteCompiler(localCompiler *LocalCompiler, serverHostPort string, workingDir string) (*RemoteCompiler, error) {
	userID, err := common.MakeUniqueUserID()
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

		headersSha256Calculator: HeaderSHA256Calculator{WorkingDir: workingDir},
	}, nil
}

func (compiler *RemoteCompiler) readHeaderAndSendSHA256OrBody(path string, mtime int64, index int32, wg *common.WaitGroupWithError) {
	headerSha256, err := compiler.headersSha256Calculator.CalcSHA256(path, mtime)
	if err != nil {
		wg.Done(err)
		return
	}
	reply, err := compiler.grpcClient.Client.SendHeaderSHA256(
		compiler.grpcClient.CallContext,
		&pb.SendHeaderSHA256Request{
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
		wg.Done(fmt.Errorf("Can't open header %q for sending: %v", path, err))
		return
	}
	defer file.Close()

	stream, err := compiler.grpcClient.Client.SendHeader(compiler.grpcClient.CallContext)
	if err != nil {
		wg.Done(fmt.Errorf("Can't open grpc stream: %v", err))
		return
	}

	err = stream.Send(&pb.SendHeaderRequest{
		Chunk: &pb.SendHeaderRequest_Metadata{
			Metadata: &pb.SendHeaderRequest_HeaderMetadata{
				SessionID:   compiler.sessionID,
				HeaderIndex: index,
			},
		},
	})
	if err != nil {
		wg.Done(fmt.Errorf("Can't send header metadata: %v", err))
		return
	}

	var buffer [256 * 1024]byte
	for {
		n, err := file.Read(buffer[:])
		if err == io.EOF {
			break
		}
		if err != nil {
			wg.Done(fmt.Errorf("Can't read header %q: %v", path, err))
			return
		}
		err = stream.Send(&pb.SendHeaderRequest{
			Chunk: &pb.SendHeaderRequest_HeaderBodyChunk{
				HeaderBodyChunk: buffer[:n],
			},
		})
		if err == io.EOF {
			break
		}
	}

	if _, err = stream.CloseAndRecv(); err != nil {
		wg.Done(fmt.Errorf("Can't send header: %v", err))
	} else {
		wg.Done(nil)
	}
}

// SetupEnvironment ...
func (compiler *RemoteCompiler) SetupEnvironment(headers []*pb.HeaderMetadata) error {
	clientCacheStream, err := compiler.grpcClient.Client.StartCompilationSession(
		compiler.grpcClient.CallContext,
		&pb.StartCompilationSessionRequest{
			UserID:          compiler.userID,
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

	wg := common.WaitGroupWithError{}
	wg.Add(len(clientCacheStream.MissedHeadersSHA256) + len(clientCacheStream.MissedHeadersFullCopy))
	for _, index := range clientCacheStream.MissedHeadersSHA256 {
		go compiler.readHeaderAndSendSHA256OrBody(headers[index].FilePath, headers[index].MTime, index, &wg)
	}
	for _, index := range clientCacheStream.MissedHeadersFullCopy {
		go compiler.readHeaderAndSend(headers[index].FilePath, index, &wg)
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
	compiler.headersSha256Calculator.Clear()
}
