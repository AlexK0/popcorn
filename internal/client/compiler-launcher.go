package client

import (
	"errors"
	"hash/fnv"
	"os"
	"path/filepath"

	pb "github.com/AlexK0/popcorn/internal/api/proto/v1"
	"github.com/AlexK0/popcorn/internal/common"
)

// ErrNoAvailableHosts ...
var ErrNoAvailableHosts = errors.New("no available hosts for connection")

func makeHeaderAsync(headerPath string, destMeta **pb.HeaderMetadata, wg *common.WaitGroupWithError) {
	headerStat, err := os.Stat(headerPath)
	if err == nil {
		*destMeta = &pb.HeaderMetadata{
			FilePath: headerPath,
			MTime:    headerStat.ModTime().UnixNano(),
		}
	}
	wg.Done(err)
}

func readHeadersMeta(headers []string) ([]*pb.HeaderMetadata, error) {
	wg := common.WaitGroupWithError{}
	wg.Add(len(headers))
	cachedHeaders := make([]*pb.HeaderMetadata, len(headers))
	for i, header := range headers {
		go makeHeaderAsync(header, &cachedHeaders[i], &wg)
	}
	err := wg.Wait()
	return cachedHeaders, err
}

func chooseServerNumber(localCompiler *LocalCompiler, hostsCount int) int {
	hasher := fnv.New32a()
	_, _ = hasher.Write([]byte(filepath.Base(localCompiler.inFile)))
	return int(hasher.Sum32()) % hostsCount
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

	headersMeta, err := readHeadersMeta(headers)
	if err != nil {
		return 0, nil, nil, err
	}

	remoteServer := settings.Servers[chooseServerNumber(localCompiler, hostsCount)]
	remoteCompiler, err := MakeRemoteCompiler(localCompiler, remoteServer)
	if err != nil {
		return 0, nil, nil, err
	}
	defer remoteCompiler.Clear()

	if err = remoteCompiler.SetupEnvironment(headersMeta); err != nil {
		return 0, nil, nil, err
	}

	return remoteCompiler.CompileSource()
}

// PerformCompilation ...
func PerformCompilation(compilerCmdLine []string, settings *Settings) (retCode int, stdout []byte, stderr []byte) {
	localCompiler := MakeLocalCompiler(compilerCmdLine)
	if localCompiler.RemoteCompilationAllowed {
		common.LogInfo("Trying remote compilaton")
		retCode, stdout, stderr, err := tryRemoteCompilation(localCompiler, settings)
		if err == nil {
			return retCode, stdout, stderr
		}
		common.LogError("Can't compile remotely:", err)
	}

	return localCompiler.CompileLocally()
}
