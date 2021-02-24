package server

import (
	"sync"

	"github.com/AlexK0/popcorn/internal/common"
)

type fileMeta struct {
	common.SHA256Struct
	mtime int64
}

// FileSHA256Cache ...
type FileSHA256Cache struct {
	table map[string]fileMeta
	mu    sync.RWMutex
}

// GetFileSHA256 ...
func (headerCache *FileSHA256Cache) GetFileSHA256(headerPath string, headerMTime int64) (common.SHA256Struct, bool) {
	headerCache.mu.RLock()
	meta, ok := headerCache.table[headerPath]
	headerCache.mu.RUnlock()
	if meta.mtime != headerMTime {
		return common.SHA256Struct{}, false
	}
	return meta.SHA256Struct, ok
}

// SetFileSHA256 ...
func (headerCache *FileSHA256Cache) SetFileSHA256(headerPath string, headerMTime int64, sha256sum common.SHA256Struct) {
	headerCache.mu.Lock()
	headerCache.table[headerPath] = fileMeta{sha256sum, headerMTime}
	headerCache.mu.Unlock()
}

// GetFilesCount ...
func (headerCache *FileSHA256Cache) GetFilesCount() uint64 {
	headerCache.mu.RLock()
	elements := len(headerCache.table)
	headerCache.mu.RUnlock()
	return uint64(elements)
}

// UserCache ...
type UserCache struct {
	users map[common.SHA256Struct]*FileSHA256Cache
	mu    sync.RWMutex
}

// MakeUserCache ...
func MakeUserCache() *UserCache {
	return &UserCache{
		users: make(map[common.SHA256Struct]*FileSHA256Cache, 1024),
	}
}

// GetFilesCache ...
func (userCache *UserCache) GetFilesCache(userID common.SHA256Struct) *FileSHA256Cache {
	userCache.mu.RLock()
	headerCache := userCache.users[userID]
	userCache.mu.RUnlock()

	if headerCache != nil {
		return headerCache
	}

	newHeaderCache := &FileSHA256Cache{
		table: make(map[string]fileMeta, 1024),
	}

	userCache.mu.Lock()
	headerCache = userCache.users[userID]
	if headerCache == nil {
		userCache.users[userID] = newHeaderCache
		headerCache = newHeaderCache
	}
	userCache.mu.Unlock()
	return headerCache
}

// GetCachesCount ...
func (userCache *UserCache) GetCachesCount() uint64 {
	userCache.mu.RLock()
	clientsCount := len(userCache.users)
	userCache.mu.RUnlock()
	return uint64(clientsCount)
}
