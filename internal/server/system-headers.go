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
func (systemHeaderCache *SystemHeaderCache) GetSystemHeaderSHA256AndSize(headerPath string) (common.SHA256Struct, int64) {
	if !strings.HasPrefix(headerPath, "/usr/") {
		return common.SHA256Struct{}, 0
	}

	info, err := os.Stat(headerPath)
	if err != nil {
		return common.SHA256Struct{}, 0
	}

	mtime := info.ModTime().UnixNano()
	fileSize := info.Size()
	if sha256sum, ok := systemHeaderCache.systemHeaders.GetFileSHA256(headerPath, mtime, fileSize); ok {
		return sha256sum, fileSize
	}

	sha256sum, err := common.GetFileSHA256(headerPath)
	if err == nil {
		systemHeaderCache.systemHeaders.SetFileSHA256(headerPath, mtime, fileSize, sha256sum)
	}
	return sha256sum, fileSize
}

// GetSystemHeadersCount ...
func (systemHeaderCache *SystemHeaderCache) GetSystemHeadersCount() int64 {
	return systemHeaderCache.systemHeaders.Count()
}
