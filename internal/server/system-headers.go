package server

import (
	"os"
	"strings"

	"github.com/AlexK0/popcorn/internal/common"
)

// SystemHeaderCache ...
type SystemHeaderCache struct {
	systemHeaders FileSHA256Cache
}

// MakeSystemHeaderCache ...
func MakeSystemHeaderCache() *SystemHeaderCache {
	return &SystemHeaderCache{
		systemHeaders: FileSHA256Cache{table: make(map[string]fileMeta, 512)},
	}
}

// GetSystemHeaderSHA256 ...
func (systemHeaderCache *SystemHeaderCache) IsSystemHeader(headerPath string, headerFileSize int64, headerSHA256 common.SHA256Struct) bool {
	if !strings.HasPrefix(headerPath, "/usr/") {
		return false
	}

	info, err := os.Stat(headerPath)
	if err != nil {
		return false
	}

	fileSize := info.Size()
	if fileSize != headerFileSize {
		return false
	}

	mtime := info.ModTime().UnixNano()
	sha256sum, ok := systemHeaderCache.systemHeaders.GetFileSHA256(headerPath, mtime, fileSize)
	if !ok {
		if sha256sum, err = common.GetFileSHA256(headerPath); err == nil {
			systemHeaderCache.systemHeaders.SetFileSHA256(headerPath, mtime, fileSize, sha256sum)
		}
	}
	return sha256sum == headerSHA256
}

// GetSystemHeadersCount ...
func (systemHeaderCache *SystemHeaderCache) GetSystemHeadersCount() int64 {
	return systemHeaderCache.systemHeaders.Count()
}
