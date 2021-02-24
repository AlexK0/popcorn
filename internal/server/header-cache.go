package server

import (
	"sync"

	"github.com/AlexK0/popcorn/internal/common"
)

type headerMeta struct {
	common.SHA256Struct
	mtime int64
}

// HeaderCache ...
type HeaderCache struct {
	headersMeta map[string]headerMeta
	mu          sync.RWMutex
}

// GetHeaderSHA256 ...
func (headerCache *HeaderCache) GetHeaderSHA256(headerPath string, headerMTime int64) (common.SHA256Struct, bool) {
	headerCache.mu.RLock()
	meta, ok := headerCache.headersMeta[headerPath]
	headerCache.mu.RUnlock()
	if meta.mtime != headerMTime {
		return common.SHA256Struct{}, false
	}
	return meta.SHA256Struct, ok
}

// SetHeaderSHA256 ...
func (headerCache *HeaderCache) SetHeaderSHA256(headerPath string, headerMTime int64, sha256sum common.SHA256Struct) {
	headerCache.mu.Lock()
	headerCache.headersMeta[headerPath] = headerMeta{sha256sum, headerMTime}
	headerCache.mu.Unlock()
}

// GetHeadersCount ...
func (headerCache *HeaderCache) GetHeadersCount() uint64 {
	headerCache.mu.RLock()
	elements := len(headerCache.headersMeta)
	headerCache.mu.RUnlock()
	return uint64(elements)
}

// UserCache ...
type UserCache struct {
	users map[common.SHA256Struct]*HeaderCache
	mu    sync.RWMutex
}

// MakeUserCache ...
func MakeUserCache() *UserCache {
	return &UserCache{
		users: make(map[common.SHA256Struct]*HeaderCache, 1024),
	}
}

// GetHeaderCache ...
func (clientMap *UserCache) GetHeaderCache(clientID common.SHA256Struct) *HeaderCache {
	clientMap.mu.RLock()
	headerCache := clientMap.users[clientID]
	clientMap.mu.RUnlock()

	if headerCache != nil {
		return headerCache
	}

	newHeaderCache := &HeaderCache{
		headersMeta: make(map[string]headerMeta, 1024),
	}

	clientMap.mu.Lock()
	headerCache = clientMap.users[clientID]
	if headerCache == nil {
		clientMap.users[clientID] = newHeaderCache
		headerCache = newHeaderCache
	}
	clientMap.mu.Unlock()
	return headerCache
}

// GetCachesCount ...
func (clientMap *UserCache) GetCachesCount() uint64 {
	clientMap.mu.RLock()
	clientsCount := len(clientMap.users)
	clientMap.mu.RUnlock()
	return uint64(clientsCount)
}
