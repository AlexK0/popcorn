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
func (systemHeaderCache *SystemHeaderCache) GetSystemHeaderSHA256(headerPath string) common.SHA256Struct {
	if !strings.HasPrefix(headerPath, "/usr/") {
		return common.SHA256Struct{}
	}

	info, err := os.Stat(headerPath)
	if err != nil {
		return common.SHA256Struct{}
	}

	mtime := info.ModTime().UnixNano()
	if sha256sum, ok := systemHeaderCache.systemHeaders.GetFileSHA256(headerPath, mtime); ok {
		return sha256sum
	}

	sha256sum, err := common.GetFileSHA256(headerPath)
	if err == nil {
		systemHeaderCache.systemHeaders.SetFileSHA256(headerPath, mtime, sha256sum)
	}
	return sha256sum
}

// GetSystemHeadersCount ...
func (systemHeaderCache *SystemHeaderCache) GetSystemHeadersCount() int64 {
	return systemHeaderCache.systemHeaders.Count()
}
