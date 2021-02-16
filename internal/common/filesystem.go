package common

import (
	"bytes"
	"errors"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// ReadFile ...
func ReadFile(filePath string) (*bytes.Buffer, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var fileContentBuffer bytes.Buffer
	if _, err = io.Copy(&fileContentBuffer, file); err != nil {
		return nil, err
	}
	return &fileContentBuffer, nil
}

// WriteFile ...
func WriteFile(fullPath string, fileContent []byte) error {
	directory, fileName := path.Split(fullPath)
	if err := os.MkdirAll(directory, os.ModePerm); err != nil {
		return err
	}

	tmpFile, err := ioutil.TempFile(directory, fileName)
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

// ErrNoMacAddress ...
var ErrNoMacAddress = errors.New("can't find mac address")

// SearchMacAddress ...
func SearchMacAddress() (*bytes.Buffer, error) {
	filesWithMac, _ := filepath.Glob("/sys/class/net/*/address")
	sort.Strings(filesWithMac)
	for _, macFile := range filesWithMac {
		if strings.HasPrefix(macFile, "/sys/class/net/eth") || strings.HasPrefix(macFile, "/sys/class/net/wlp") {
			return ReadFile(macFile)
		}
	}

	return nil, ErrNoMacAddress
}

// NormalizePaths ...
func NormalizePaths(paths []string) []string {
	usedPaths := make(map[string]bool)
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
