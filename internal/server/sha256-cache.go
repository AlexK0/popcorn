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

func (headerCache *FileSHA256Cache) GetFileSHA256(headerPath string, headerMTime int64, fileSize int64) (common.SHA256Struct, bool) {
	headerCache.mu.RLock()
	meta, ok := headerCache.table[headerPath]
	headerCache.mu.RUnlock()
	if meta.mtime != headerMTime || meta.fileSize != fileSize {
		return common.SHA256Struct{}, false
	}
	return meta.SHA256Struct, ok
}

func (headerCache *FileSHA256Cache) SetFileSHA256(headerPath string, headerMTime int64, fileSize int64, sha256sum common.SHA256Struct) {
	headerCache.mu.Lock()
	headerCache.table[headerPath] = fileMeta{sha256sum, headerMTime, fileSize}
	headerCache.mu.Unlock()
}

func (headerCache *FileSHA256Cache) Count() int64 {
	headerCache.mu.RLock()
	elements := len(headerCache.table)
	headerCache.mu.RUnlock()
	return int64(elements)
}
