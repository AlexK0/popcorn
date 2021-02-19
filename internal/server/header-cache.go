package server

import (
	"os"
	"strings"
	"sync"
	"time"

	"github.com/AlexK0/popcorn/internal/common"
)

type headerMeta struct {
	sha256sum string
	mtime     int64
}

// ClientHeaderCache ...
type ClientHeaderCache struct {
	headersMeta map[string]headerMeta
	mu          sync.RWMutex
}

// GetHeaderSHA256 ...
func (headerCache *ClientHeaderCache) GetHeaderSHA256(headerPath string, headerMTime int64) string {
	headerCache.mu.RLock()
	meta := headerCache.headersMeta[headerPath]
	headerCache.mu.RUnlock()
	if meta.mtime != headerMTime {
		return ""
	}
	return meta.sha256sum
}

// SetHeaderSHA256 ...
func (headerCache *ClientHeaderCache) SetHeaderSHA256(headerPath string, headerMTime int64, sha256sum string) {
	meta := headerMeta{sha256sum, headerMTime}
	headerCache.mu.Lock()
	headerCache.headersMeta[headerPath] = meta
	headerCache.mu.Unlock()
}

// GetHeadersCount ...
func (headerCache *ClientHeaderCache) GetHeadersCount() uint64 {
	headerCache.mu.RLock()
	elements := len(headerCache.headersMeta)
	headerCache.mu.RUnlock()
	return uint64(elements)
}

type clientKey struct {
	machineID string
	mac       string
	userName  string
}

// ClientCacheMap ...
type ClientCacheMap struct {
	clients map[clientKey]*ClientHeaderCache
	mu      sync.RWMutex
}

// MakeClientCacheMap ...
func MakeClientCacheMap() *ClientCacheMap {
	return &ClientCacheMap{
		clients: make(map[clientKey]*ClientHeaderCache, 1024),
	}
}

// GetHeaderCache ...
func (clientMap *ClientCacheMap) GetHeaderCache(machineID string, mac string, userName string) *ClientHeaderCache {
	key := clientKey{machineID, mac, userName}
	clientMap.mu.RLock()
	headerCache := clientMap.clients[key]
	clientMap.mu.RUnlock()

	if headerCache != nil {
		return headerCache
	}

	newHeaderCache := &ClientHeaderCache{
		headersMeta: make(map[string]headerMeta, 1024),
	}

	clientMap.mu.Lock()
	headerCache = clientMap.clients[key]
	if headerCache == nil {
		clientMap.clients[key] = newHeaderCache
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
	sha256sum string
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
func (processingHeaders *ProcessingHeadersMap) StartHeaderProcessing(headerPath string, sha256sum string) bool {
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
func (processingHeaders *ProcessingHeadersMap) ForceStartHeaderProcessing(headerPath string, sha256sum string) {
	key := processingHeaderKey{headerPath, sha256sum}
	now := time.Now()
	processingHeaders.mu.Lock()
	processingHeaders.headers[key] = now
	processingHeaders.mu.Unlock()
}

// FinishHeaderProcessing ...
func (processingHeaders *ProcessingHeadersMap) FinishHeaderProcessing(headerPath string, sha256sum string) {
	key := processingHeaderKey{headerPath, sha256sum}
	processingHeaders.mu.Lock()
	delete(processingHeaders.headers, key)
	processingHeaders.mu.Unlock()
}

// SystemHeaderCache ...
type SystemHeaderCache struct {
	cache ClientHeaderCache
}

// MakeSystemHeaderCache ...
func MakeSystemHeaderCache() *SystemHeaderCache {
	return &SystemHeaderCache{
		cache: ClientHeaderCache{headersMeta: make(map[string]headerMeta, 512)},
	}
}

// GetSystemHeaderSHA256 ...
func (systemHeaderCache *SystemHeaderCache) GetSystemHeaderSHA256(headerPath string) string {
	if !strings.HasPrefix(headerPath, "/usr/") {
		return ""
	}

	info, err := os.Stat(headerPath)
	if err != nil {
		return ""
	}

	mtime := info.ModTime().UnixNano()
	if sha256sum := systemHeaderCache.cache.GetHeaderSHA256(headerPath, mtime); len(sha256sum) != 0 {
		return sha256sum
	}

	sha256sum, _ := common.GetFileSHA256(headerPath)
	if len(sha256sum) != 0 {
		systemHeaderCache.cache.SetHeaderSHA256(headerPath, mtime, sha256sum)
	}
	return sha256sum
}

// GetSystemHeadersCacheSize ...
func (systemHeaderCache *SystemHeaderCache) GetSystemHeadersCacheSize() uint64 {
	return systemHeaderCache.cache.GetHeadersCount()
}
