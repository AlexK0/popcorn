package client

import (
	"io"
	"io/ioutil"
	"os"
	"os/user"
	"strings"

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

	clientID *pb.ClientIdentifier
}

func makeClientID() (*pb.ClientIdentifier, error) {
	machineID, err := ioutil.ReadFile("/etc/machine-id")
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
		MachineID:  strings.TrimSpace(string(machineID)),
		MacAddress: strings.ReplaceAll(strings.TrimSpace(string(mac)), ":", "-"),
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

	grpcClient, err := MakeGRPCClient(serverHostPort)
	if err != nil {
		return nil, err
	}

	return &RemoteCompiler{
		name:          localCompiler.name,
		inFile:        localCompiler.inFile,
		outFile:       localCompiler.outFile,
		remoteCmdArgs: localCompiler.MakeRemoteCmd("="),

		grpcClient: grpcClient,
		clientID:   clientID,
	}, nil
}

func (compiler *RemoteCompiler) copyHeaderAsync(headersFullCopy *pb.HeaderFullData, errorChannel chan<- error) {
	_, err := compiler.grpcClient.Client.CopyHeader(
		compiler.grpcClient.CallContext, &pb.CopyHeaderRequest{
			ClientID: compiler.clientID,
			Header:   headersFullCopy,
		})
	errorChannel <- err
}

// SetupEnvironment ...
func (compiler *RemoteCompiler) SetupEnvironment(headers []*pb.HeaderClientMeta) error {
	clientCacheStream, err := compiler.grpcClient.Client.CopyHeadersFromClientCache(
		compiler.grpcClient.CallContext, &pb.CopyHeadersFromClientCacheRequest{
			ClientID:                   compiler.clientID,
			ClientHeaders:              headers,
			ClearEnvironmentBeforeCopy: true,
		})
	if err != nil {
		return err
	}

	copyHadersChannel := make(chan error)
	copyHadersCount := 0

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
			copyHadersCount++
			go compiler.copyHeaderAsync(fullHeader, copyHadersChannel)
		} else {
			headersFullForGlobalCache = append(headersFullForGlobalCache, fullHeader)
			headersForGlobalCache = append(headersForGlobalCache, fullHeader.GlobalMeta)
		}
	}

	globalCacheStream, err := compiler.grpcClient.Client.CopyHeadersFromGlobalCache(
		compiler.grpcClient.CallContext, &pb.CopyHeadersFromGlobalCacheRequest{
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

		copyHadersCount++
		go compiler.copyHeaderAsync(headersFullForGlobalCache[int(copyRes.MissedHeaderIndex)], copyHadersChannel)
	}

	for i := 0; i < copyHadersCount; i++ {
		if copyResultErr := <-copyHadersChannel; copyResultErr != nil {
			err = copyResultErr
		}
	}

	return err
}

// CompileSource ...
func (compiler *RemoteCompiler) CompileSource() (retCode int, stdout []byte, stderr []byte, err error) {
	sourceBody, err := ioutil.ReadFile(compiler.inFile)
	if err != nil {
		return 0, nil, nil, err
	}
	res, err := compiler.grpcClient.Client.CompileSource(
		compiler.grpcClient.CallContext, &pb.CompileSourceRequest{
			ClientID:                   compiler.clientID,
			FilePath:                   compiler.inFile,
			Compiler:                   compiler.name,
			CompilerArgs:               compiler.remoteCmdArgs,
			SourceBody:                 sourceBody,
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
	compiler.grpcClient.Clear()
}
