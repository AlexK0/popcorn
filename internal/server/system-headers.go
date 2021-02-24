package server

import (
	"os"
	"strings"

	"github.com/AlexK0/popcorn/internal/common"
)

// SystemHeaderCache ...
type SystemHeaderCache struct {
	systemHeaders HeaderCache
}

// MakeSystemHeaderCache ...
func MakeSystemHeaderCache() *SystemHeaderCache {
	return &SystemHeaderCache{
		systemHeaders: HeaderCache{headersMeta: make(map[string]headerMeta, 512)},
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
	if sha256sum, ok := systemHeaderCache.systemHeaders.GetHeaderSHA256(headerPath, mtime); ok {
		return sha256sum
	}

	sha256sum, err := common.GetFileSHA256(headerPath)
	if err == nil {
		systemHeaderCache.systemHeaders.SetHeaderSHA256(headerPath, mtime, sha256sum)
	}
	return sha256sum
}

// GetSystemHeadersCacheSize ...
func (systemHeaderCache *SystemHeaderCache) GetSystemHeadersCacheSize() uint64 {
	return systemHeaderCache.systemHeaders.GetHeadersCount()
}
