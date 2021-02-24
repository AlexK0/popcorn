package server

import (
	"os"
	"strings"
	"sync"
	"time"

	"github.com/AlexK0/popcorn/internal/common"
)

type headerMeta struct {
	sha256sum common.SHA256Struct
	mtime     int64
}

// UserHeaderCache ...
type UserHeaderCache struct {
	headersMeta map[string]headerMeta
	mu          sync.RWMutex
}

// GetHeaderSHA256 ...
func (headerCache *UserHeaderCache) GetHeaderSHA256(headerPath string, headerMTime int64) (common.SHA256Struct, bool) {
	headerCache.mu.RLock()
	meta, ok := headerCache.headersMeta[headerPath]
	headerCache.mu.RUnlock()
	if meta.mtime != headerMTime {
		return common.SHA256Struct{}, false
	}
	return meta.sha256sum, ok
}

// SetHeaderSHA256 ...
func (headerCache *UserHeaderCache) SetHeaderSHA256(headerPath string, headerMTime int64, sha256sum common.SHA256Struct) {
	headerCache.mu.Lock()
	headerCache.headersMeta[headerPath] = headerMeta{sha256sum, headerMTime}
	headerCache.mu.Unlock()
}

// GetHeadersCount ...
func (headerCache *UserHeaderCache) GetHeadersCount() uint64 {
	headerCache.mu.RLock()
	elements := len(headerCache.headersMeta)
	headerCache.mu.RUnlock()
	return uint64(elements)
}

// ClientCacheMap ...
type ClientCacheMap struct {
	clients map[common.SHA256Struct]*UserHeaderCache
	mu      sync.RWMutex
}

// MakeClientCacheMap ...
func MakeClientCacheMap() *ClientCacheMap {
	return &ClientCacheMap{
		clients: make(map[common.SHA256Struct]*UserHeaderCache, 1024),
	}
}

// GetHeaderCache ...
func (clientMap *ClientCacheMap) GetHeaderCache(clientID common.SHA256Struct) *UserHeaderCache {
	clientMap.mu.RLock()
	headerCache := clientMap.clients[clientID]
	clientMap.mu.RUnlock()

	if headerCache != nil {
		return headerCache
	}

	newHeaderCache := &UserHeaderCache{
		headersMeta: make(map[string]headerMeta, 1024),
	}

	clientMap.mu.Lock()
	headerCache = clientMap.clients[clientID]
	if headerCache == nil {
		clientMap.clients[clientID] = newHeaderCache
		headerCache = newHeaderCache
	}
	clientMap.mu.Unlock()
	return headerCache
}

// GetCachesCount ...
func (clientMap *ClientCacheMap) GetCachesCount() uint64 {
	clientMap.mu.RLock()
	clientsCount := len(clientMap.clients)
	clientMap.mu.RUnlock()
	return uint64(clientsCount)
}

type processingHeaderKey struct {
	path      string
	sha256sum common.SHA256Struct
}

// ProcessingHeadersMap ...
type ProcessingHeadersMap struct {
	headers map[processingHeaderKey]time.Time
	mu      sync.Mutex
}

// MakeProcessingHeaders ...
func MakeProcessingHeaders() *ProcessingHeadersMap {
	return &ProcessingHeadersMap{
		headers: make(map[processingHeaderKey]time.Time, 1024),
	}
}

// StartHeaderProcessing ...
func (processingHeaders *ProcessingHeadersMap) StartHeaderProcessing(headerPath string, sha256sum common.SHA256Struct) bool {
	key := processingHeaderKey{headerPath, sha256sum}
	now := time.Now()
	started := false
	processingHeaders.mu.Lock()
	processingStartTime, alreadyStarted := processingHeaders.headers[key]
	// TODO Why 5 second?
	if !alreadyStarted || now.Sub(processingStartTime) > time.Second*5 {
		processingHeaders.headers[key] = now
		started = true
	}
	processingHeaders.mu.Unlock()
	return started
}

// ForceStartHeaderProcessing ...
func (processingHeaders *ProcessingHeadersMap) ForceStartHeaderProcessing(headerPath string, sha256sum common.SHA256Struct) {
	key := processingHeaderKey{headerPath, sha256sum}
	now := time.Now()
	processingHeaders.mu.Lock()
	processingHeaders.headers[key] = now
	processingHeaders.mu.Unlock()
}

// FinishHeaderProcessing ...
func (processingHeaders *ProcessingHeadersMap) FinishHeaderProcessing(headerPath string, sha256sum common.SHA256Struct) {
	key := processingHeaderKey{headerPath, sha256sum}
	processingHeaders.mu.Lock()
	delete(processingHeaders.headers, key)
	processingHeaders.mu.Unlock()
}

// SystemHeaderCache ...
type SystemHeaderCache struct {
	cache UserHeaderCache
}

// MakeSystemHeaderCache ...
func MakeSystemHeaderCache() *SystemHeaderCache {
	return &SystemHeaderCache{
		cache: UserHeaderCache{headersMeta: make(map[string]headerMeta, 512)},
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
	if sha256sum, ok := systemHeaderCache.cache.GetHeaderSHA256(headerPath, mtime); ok {
		return sha256sum
	}

	sha256sum, err := common.GetFileSHA256(headerPath)
	if err == nil {
		systemHeaderCache.cache.SetHeaderSHA256(headerPath, mtime, sha256sum)
	}
	return sha256sum
}

// GetSystemHeadersCacheSize ...
func (systemHeaderCache *SystemHeaderCache) GetSystemHeadersCacheSize() uint64 {
	return systemHeaderCache.cache.GetHeadersCount()
}
