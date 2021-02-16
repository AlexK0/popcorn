package client

import (
	"errors"
	"hash/fnv"

	pb "github.com/AlexK0/popcorn/internal/api/proto/v1"
	"github.com/AlexK0/popcorn/internal/common"
)

// ErrNoAvailableHosts ...
var ErrNoAvailableHosts = errors.New("no available hosts for connection")

type asyncHeaderResult struct {
	headerMeta *pb.HeaderClientMeta
	err        error
}

func makeHeaderAsync(header string, headerChannel chan<- asyncHeaderResult) {
	var result asyncHeaderResult
	result.headerMeta, result.err = MakeClientHeaderMeta(header)
	headerChannel <- result
}

func readHeadersMeta(headers []string) ([]*pb.HeaderClientMeta, error) {
	headerChannel := make(chan asyncHeaderResult)
	for _, header := range headers {
		go makeHeaderAsync(header, headerChannel)
	}

	cachedHeaders := make([]*pb.HeaderClientMeta, len(headers))
	var err error
	for i := range headers {
		result := <-headerChannel
		cachedHeaders[i] = result.headerMeta
		if result.err != nil {
			err = result.err
		}
	}
	if err != nil {
		return nil, err
	}
	return cachedHeaders, nil
}

func chooseServerNumber(localCompiler *LocalCompiler, hostsCount int) uint64 {
	hasher := fnv.New64()
	_, _ = hasher.Write([]byte(localCompiler.inFile))
	return hasher.Sum64() % uint64(hostsCount)
}

func tryRemoteCompilation(localCompiler *LocalCompiler, settings *Settings) (retCode int, stdout []byte, stderr []byte, err error) {
	hostsCount := len(settings.Servers)
	if hostsCount == 0 {
		return 0, nil, nil, ErrNoAvailableHosts
	}

	headers, err := localCompiler.CollectHeadersAndUpdateIncludeDirs()
	if err != nil {
		return 0, nil, nil, err
	}

	cachedHeaders, err := readHeadersMeta(headers)
	if err != nil {
		return 0, nil, nil, err
	}

	remoteServer := settings.Servers[chooseServerNumber(localCompiler, hostsCount)]
	remoteCompiler, err := MakeRemoteCompiler(localCompiler, remoteServer)
	if err != nil {
		return 0, nil, nil, err
	}
	defer remoteCompiler.Clear()

	if err = remoteCompiler.SetupEnvironment(cachedHeaders); err != nil {
		return 0, nil, nil, err
	}

	return remoteCompiler.CompileSource()
}

// PerformCompilation ...
func PerformCompilation(compilerCmdLine []string, settings *Settings) (retCode int, stdout []byte, stderr []byte) {
	localCompiler := MakeLocalCompiler(compilerCmdLine)
	if localCompiler.RemoteCompilationAllowed {
		common.LogInfo("Trying remote compilaton")
		if retCode, stdout, stderr, err := tryRemoteCompilation(localCompiler, settings); err == nil {
			return retCode, stdout, stderr
		} else {
			common.LogError("Can't compile remotely:", err)
		}
	}

	return localCompiler.CompileLocally()
}
