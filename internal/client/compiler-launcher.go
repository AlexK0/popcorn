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

func makeFileMetaAsync(filePath string, destMeta **pb.FileMetadata, wg *common.WaitGroupWithError) {
	headerStat, err := os.Stat(filePath)
	if err == nil {
		*destMeta = &pb.FileMetadata{
			FilePath: filePath,
			MTime:    headerStat.ModTime().UnixNano(),
			FileSize: headerStat.Size(),
		}
	}
	wg.Done(err)
}

func readFilesMeta(files []string) ([]*pb.FileMetadata, error) {
	wg := common.WaitGroupWithError{}
	wg.Add(len(files))
	filesMeta := make([]*pb.FileMetadata, len(files))
	for i, file := range files {
		go makeFileMetaAsync(file, &filesMeta[i], &wg)
	}
	err := wg.Wait()
	return filesMeta, err
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

	files, err := localCompiler.CollectFilesAndUpdateIncludeDirs()
	if err != nil {
		return 0, nil, nil, err
	}

	filesMeta, err := readFilesMeta(files)
	if err != nil {
		return 0, nil, nil, err
	}

	remoteServer := settings.Servers[chooseServerNumber(localCompiler, hostsCount)]
	remoteCompiler, err := MakeRemoteCompiler(localCompiler, remoteServer)
	if err != nil {
		return 0, nil, nil, err
	}
	defer remoteCompiler.Clear()

	if err = remoteCompiler.SetupEnvironment(filesMeta); err != nil {
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
