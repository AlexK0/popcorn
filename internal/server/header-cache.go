package server

import "sync"

type headerKey struct {
	path  string
	mTime int64
}

// ClientHeaderCache ...
type ClientHeaderCache struct {
	headersSHA256 map[headerKey]string
	mu            sync.RWMutex
}

type clientKey struct {
	machineID string
	mac       string
	userName  string
}

type clientCacheMap struct {
	clients map[clientKey]*ClientHeaderCache
	mu      sync.RWMutex
}

var clientMap clientCacheMap

func init() {
	clientMap.clients = make(map[clientKey]*ClientHeaderCache)
}

// GetClientHeaderCache ...
func GetClientHeaderCache(machineID string, mac string, userName string) *ClientHeaderCache {
	key := clientKey{machineID, mac, userName}
	clientMap.mu.RLock()
	headerCache := clientMap.clients[key]
	clientMap.mu.RUnlock()

	if headerCache != nil {
		return headerCache
	}

	newHeaderCache := &ClientHeaderCache{
		headersSHA256: make(map[headerKey]string),
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

// GetClientCachesCount ...
func GetClientCachesCount() uint64 {
	clientMap.mu.RLock()
	clientsCount := len(clientMap.clients)
	clientMap.mu.RUnlock()
	return uint64(clientsCount)
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
