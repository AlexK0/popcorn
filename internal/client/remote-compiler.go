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

func (compiler *RemoteCompiler) readHeaderSHA256AndSend(path string, index int32) (bool, error) {
	headerSha256, err := common.GetFileSHA256(path)
	if err != nil {
		return false, err
	}
	reply, err := compiler.grpcClient.Client.SendHeaderSHA256(
		compiler.grpcClient.CallContext,
		&pb.SendHeaderSHA256Request{
			SessionID:    compiler.sessionID,
			HeaderIndex:  index,
			HeaderSHA256: common.SHA256StructToSHA256Message(headerSha256),
		})
	if err != nil {
		return false, err
	}
	return reply.FullCopyRequired, nil
}

func (compiler *RemoteCompiler) readHeaderAndSend(path string, index int32) error {
	headerBody, err := ioutil.ReadFile(path)
	if err == nil {
		_, err = compiler.grpcClient.Client.SendHeader(
			compiler.grpcClient.CallContext,
			&pb.SendHeaderRequest{
				SessionID:   compiler.sessionID,
				HeaderIndex: index,
				HeaderBody:  headerBody,
			})
	}
	return err
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

	fullCopyRequired := clientCacheStream.MissedHeadersFullCopy
	for _, index := range clientCacheStream.MissedHeadersSHA256 {
		needFull, err := compiler.readHeaderSHA256AndSend(headers[index].FilePath, index)
		if err != nil {
			return err
		}
		if needFull {
			fullCopyRequired = append(fullCopyRequired, index)
		}
	}

	for _, index := range fullCopyRequired {
		if err = compiler.readHeaderAndSend(headers[index].FilePath, index); err != nil {
			return err
		}
	}
	return nil
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
