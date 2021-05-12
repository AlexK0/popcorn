package common

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
)

func OpenTempFile(fullPath string) (f *os.File, err error) {
	directory, fileName := filepath.Split(fullPath)
	if err := os.MkdirAll(directory, os.ModePerm); err != nil {
		return nil, err
	}
	return ioutil.TempFile(directory, fileName)
}

// WriteFile ...
func WriteFile(fullPath string, fileContent []byte) error {
	tmpFile, err := OpenTempFile(fullPath)
	if err != nil {
		return err
	}
	_, err = tmpFile.Write(fileContent)
	tmpFile.Close()
	if err != nil {
		return err
	}
	return os.Rename(tmpFile.Name(), fullPath)
}

// NormalizePaths ...
func NormalizePaths(paths []string) []string {
	usedPaths := make(map[string]bool, len(paths))
	result := make([]string, 0, len(paths))
	for _, path := range paths {
		newPath, err := filepath.EvalSymlinks(path)
		if err == nil {
			path = newPath
		}
		newPath, err = filepath.Abs(path)
		if err == nil {
			path = newPath
		}
		if !usedPaths[path] {
			result = append(result, path)
			usedPaths[path] = true
		}
	}
	return result
}

func TransferFileByChunks(path string, transferCallback func(chunk []byte) error) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("Can't open file %q: %v", path, err)
	}
	defer file.Close()

	var buffer [128 * 1024]byte
	for {
		n, err := file.Read(buffer[:])
		if err == io.EOF {
			err = nil
			n = 0
		}
		if err != nil {
			return fmt.Errorf("Can't read file %q: %v", path, err)
		}
		if err = transferCallback(buffer[:n]); err != nil {
			return fmt.Errorf("Can't transfer file %q: %v", path, err)
		}
		if n == 0 {
			return nil
		}
	}
}
