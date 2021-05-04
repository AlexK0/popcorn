package client

import (
	"io/ioutil"

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

	needCloseSession bool
}

// MakeRemoteCompiler ...
func MakeRemoteCompiler(localCompiler *LocalCompiler, serverHostPort string) (*RemoteCompiler, error) {
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
	}, nil
}

func (compiler *RemoteCompiler) readHeaderAndSendSHA256OrBody(path string, index int32, wg *common.WaitGroupWithError, fullRequired chan<- int32) {
	headerSha256, err := common.GetFileSHA256(path)
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
		fullRequired <- index
	}
	wg.Done(err)
}

// func (compiler *RemoteCompiler) readHeaderAndSend(path string, index int32) error {
// 	headerBody, err := ioutil.ReadFile(path)
// 	if err == nil {
// 		_, err = compiler.grpcClient.Client.SendHeader(
// 			compiler.grpcClient.CallContext,
// 			&pb.SendHeaderRequest{
// 				SessionID:   compiler.sessionID,
// 				HeaderIndex: index,
// 				HeaderBody:  headerBody,
// 			})
// 	}
// 	return err
// }

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
	wg.Add(len(clientCacheStream.MissedHeadersSHA256))
	fullCopyRequired := make(chan int32, len(clientCacheStream.MissedHeadersSHA256))
	for _, index := range clientCacheStream.MissedHeadersSHA256 {
		go compiler.readHeaderAndSendSHA256OrBody(headers[index].FilePath, index, &wg, fullCopyRequired)
	}

	common.WriteFile(compiler.outFile, []byte(compiler.inFile))

	return wg.Wait() // REMOVE ME
	// for _, index := range clientCacheStream.MissedHeadersFullCopy {
	// 	if err = compiler.readHeaderAndSend(headers[index].FilePath, index); err != nil {
	// 		break
	// 	}
	// }
	// if err := wg.Wait(); err != nil {
	// 	return err
	// }
	// if err != nil {
	// 	return err
	// }
	// close(fullCopyRequired)

	// for index := range fullCopyRequired {
	// 	if err = compiler.readHeaderAndSend(headers[index].FilePath, index); err != nil {
	// 		return err
	// 	}
	// }
	// return nil
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
