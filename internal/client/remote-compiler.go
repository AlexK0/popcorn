package client

import (
	"context"
	"io"
	"os"
	"os/user"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/encoding/gzip"

	pb "github.com/AlexK0/popcorn/internal/api/proto/v1"
	"github.com/AlexK0/popcorn/internal/common"
)

// RemoteCompiler ...
type RemoteCompiler struct {
	name          string
	inFile        string
	outFile       string
	remoteCmdArgs []string

	connection *grpc.ClientConn
	context    context.Context
	cancelFunc context.CancelFunc
	client     pb.CompilationServiceClient

	clientID *pb.ClientIdentifier
}

func makeClientID() (*pb.ClientIdentifier, error) {
	machineID, err := common.ReadFile("/etc/machine-id")
	if err != nil {
		return nil, err
	}

	mac, err := common.SearchMacAddress()
	if err != nil {
		return nil, err
	}

	user, err := user.Current()
	if err != nil {
		return nil, err
	}

	return &pb.ClientIdentifier{
		MachineID:  strings.TrimSpace(machineID.String()),
		MacAddress: strings.ReplaceAll(strings.TrimSpace(mac.String()), ":", "-"),
		UserName:   strings.Join([]string{user.Username, user.Uid}, "-"),
		Pid:        int32(os.Getpid()),
	}, nil
}

// MakeRemoteCompiler ...
func MakeRemoteCompiler(localCompiler *LocalCompiler, serverHostPort string) (*RemoteCompiler, error) {
	clientID, err := makeClientID()
	if err != nil {
		return nil, err
	}

	connection, err := grpc.Dial(serverHostPort,
		grpc.WithInsecure(),
		grpc.WithBlock(),
		grpc.WithDefaultCallOptions(
			grpc.UseCompressor(gzip.Name),
			grpc.MaxCallRecvMsgSize(1024*1204*1024),
			grpc.MaxCallSendMsgSize(1024*1204*1024)))
	if err != nil {
		return nil, err
	}

	context, cancelFunc := context.WithTimeout(context.Background(), 10*time.Minute)
	return &RemoteCompiler{
		name:          localCompiler.name,
		inFile:        localCompiler.inFile,
		outFile:       localCompiler.outFile,
		remoteCmdArgs: localCompiler.MakeRemoteCmd("="),

		connection: connection,
		context:    context,
		cancelFunc: cancelFunc,
		client:     pb.NewCompilationServiceClient(connection),
		clientID:   clientID,
	}, nil
}

// SetupEnvironment ...
func (compiler *RemoteCompiler) SetupEnvironment(headers []*pb.HeaderClientMeta) error {
	clientCacheStream, err := compiler.client.CopyHeadersFromClientCache(
		compiler.context, &pb.CopyHeadersFromClientCacheRequest{
			ClientID:                   compiler.clientID,
			ClientHeaders:              headers,
			ClearEnvironmentBeforeCopy: true,
		})
	if err != nil {
		return err
	}

	headersFullCopy := make([]*pb.HeaderFullData, 0, len(headers))
	headersFullForGlobalCache := make([]*pb.HeaderFullData, 0, len(headers))
	headersForGlobalCache := make([]*pb.HeaderGlobalMeta, 0, len(headers))
	for {
		copyRes, err := clientCacheStream.Recv()
		if err == io.EOF {
			break
		}

		if err != nil {
			return err
		}

		fullHeader, err := MakeHeaderFullData(headers[int(copyRes.MissedHeaderIndex)])
		if err != nil {
			return err
		}
		if copyRes.FullCopyRequired {
			headersFullCopy = append(headersFullCopy, fullHeader)
		} else {
			headersFullForGlobalCache = append(headersFullForGlobalCache, fullHeader)
			headersForGlobalCache = append(headersForGlobalCache, fullHeader.GlobalMeta)
		}
	}

	globalCacheStream, err := compiler.client.CopyHeadersFromGlobalCache(
		compiler.context, &pb.CopyHeadersFromGlobalCacheRequest{
			ClientID:      compiler.clientID,
			GlobalHeaders: headersForGlobalCache,
		})
	if err != nil {
		return err
	}
	for {
		copyRes, err := globalCacheStream.Recv()
		if err == io.EOF {
			break
		}

		if err != nil {
			return err
		}
		headersFullCopy = append(headersFullCopy, headersFullForGlobalCache[int(copyRes.MissedHeaderIndex)])
	}

	_, err = compiler.client.CopyHeaders(
		compiler.context, &pb.CopyHeadersRequest{
			ClientID: compiler.clientID,
			Headers:  headersFullCopy,
		})

	return err
}

// CompileSource ...
func (compiler *RemoteCompiler) CompileSource() (retCode int, stdout []byte, stderr []byte, err error) {
	sourceBody, err := common.ReadFile(compiler.inFile)
	if err != nil {
		return 0, nil, nil, err
	}
	res, err := compiler.client.CompileSource(compiler.context, &pb.CompileSourceRequest{
		ClientID:                   compiler.clientID,
		FilePath:                   compiler.inFile,
		Compiler:                   compiler.name,
		CompilerArgs:               compiler.remoteCmdArgs,
		SourceBody:                 sourceBody.Bytes(),
		ClearEnvironmentAfterBuild: true,
	})

	if err != nil {
		return 0, nil, nil, err
	}

	if res.CompilerRetCode == 0 {
		if err = common.WriteFile(compiler.outFile, res.CompiledSource); err != nil {
			return 0, nil, nil, err
		}
	}

	return int(res.CompilerRetCode), res.CompilerStderr, res.CompilerStdout, nil
}

// Clear ...
func (compiler *RemoteCompiler) Clear() {
	if compiler.connection != nil {
		compiler.cancelFunc()
		compiler.connection.Close()

		compiler.connection = nil
		compiler.context = nil
		compiler.cancelFunc = nil
		compiler.client = nil
	}
}
