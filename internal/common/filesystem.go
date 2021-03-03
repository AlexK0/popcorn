package common

import (
	"io/ioutil"
	"os"
	"path/filepath"
)

// WriteFile ...
func WriteFile(fullPath string, fileContent []byte) error {
	directory, fileName := filepath.Split(fullPath)
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
