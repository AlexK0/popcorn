package client

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"syscall"

	"github.com/AlexK0/popcorn/internal/common"
)

type HeaderSHA256Calculator struct {
	WorkingDir string
	trash      []string
}

func (calculator *HeaderSHA256Calculator) CalcSHA256(headerPath string, mtime int64) (common.SHA256Struct, error) {
	if !filepath.IsAbs(headerPath) {
		return common.SHA256Struct{}, fmt.Errorf("Bad header path %q", headerPath)
	}
	fileLockPath := filepath.Join(calculator.WorkingDir, fmt.Sprintf("%s.%d", headerPath[1:], mtime))

	if err := os.MkdirAll(filepath.Dir(fileLockPath), os.ModePerm); err != nil {
		return common.SHA256Struct{}, fmt.Errorf("Can't create dir for file %q: %v", fileLockPath, err)
	}

	lockFile, err := os.OpenFile(fileLockPath, os.O_RDWR|os.O_CREATE, 0666)
	if err != nil {
		return common.SHA256Struct{}, fmt.Errorf("Can't open lock file %q: %v", fileLockPath, err)
	}
	defer lockFile.Close()

	calculator.trash = append(calculator.trash, fileLockPath)
	if err := syscall.FcntlFlock(lockFile.Fd(), syscall.F_SETLKW, &syscall.Flock_t{Type: syscall.F_WRLCK}); err != nil {
		return common.SHA256Struct{}, fmt.Errorf("Can't lock file %q: %v", fileLockPath, err)
	}

	sha256FromFile, err := ioutil.ReadAll(lockFile)
	if err != nil {
		return common.SHA256Struct{}, fmt.Errorf("Can't read lock file %q: %v", fileLockPath, err)
	}
	if len(sha256FromFile) != 0 {
		return common.MakeSHA256StructFromSlice(sha256FromFile), nil
	}

	headerSha256, err := common.GetFileSHA256(headerPath)
	if err != nil {
		return common.SHA256Struct{}, fmt.Errorf("Can't calculate header sha256 from file %q: %v", headerPath, err)
	}
	_, err = lockFile.Write(common.SHA256StructToBytes(headerSha256))
	if err != nil {
		return common.SHA256Struct{}, fmt.Errorf("Can't write header sha256 to file %q: %v", fileLockPath, err)
	}
	return headerSha256, nil
}

func (calculator *HeaderSHA256Calculator) Clear() {
	for _, fileLockPath := range calculator.trash {
		os.Remove(fileLockPath)
	}
}
