package server

import (
	"sync"

	"github.com/AlexK0/popcorn/internal/common"
)

type fileMeta struct {
	common.SHA256Struct
	mtime    int64
	fileSize int64
}

type FileSHA256Cache struct {
	table map[string]fileMeta
	mu    sync.RWMutex
}

func (cache *FileSHA256Cache) GetFileSHA256(headerPath string, headerMTime int64, fileSize int64) (common.SHA256Struct, bool) {
	cache.mu.RLock()
	meta, ok := cache.table[headerPath]
	cache.mu.RUnlock()
	if meta.mtime != headerMTime || meta.fileSize != fileSize {
		return common.SHA256Struct{}, false
	}
	return meta.SHA256Struct, ok
}

func (cache *FileSHA256Cache) SetFileSHA256(headerPath string, headerMTime int64, fileSize int64, sha256sum common.SHA256Struct) {
	cache.mu.Lock()
	cache.table[headerPath] = fileMeta{sha256sum, headerMTime, fileSize}
	cache.mu.Unlock()
}

func (cache *FileSHA256Cache) Count() int64 {
	cache.mu.RLock()
	elements := len(cache.table)
	cache.mu.RUnlock()
	return int64(elements)
}
