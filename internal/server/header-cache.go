package server

import (
	"sync"
	"time"
)

type headerKey struct {
	path  string
	mTime int64
}

// ClientHeaderCache ...
type ClientHeaderCache struct {
	headersSHA256 map[headerKey]string
	mu            sync.RWMutex
}

// GetHeaderSHA256 ...
func (headerCache *ClientHeaderCache) GetHeaderSHA256(headerPath string, headerMTime int64) string {
	key := headerKey{headerPath, headerMTime}
	headerCache.mu.RLock()
	sha256sum := headerCache.headersSHA256[key]
	headerCache.mu.RUnlock()
	return sha256sum
}

// SetHeaderSHA256 ...
func (headerCache *ClientHeaderCache) SetHeaderSHA256(headerPath string, headerMTime int64, sha256sum string) {
	key := headerKey{headerPath, headerMTime}
	headerCache.mu.Lock()
	headerCache.headersSHA256[key] = sha256sum
	headerCache.mu.Unlock()
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
		headersSHA256: make(map[headerKey]string, 1024),
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
