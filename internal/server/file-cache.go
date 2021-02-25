package server

import (
	"fmt"
	"os"
	"path"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/AlexK0/popcorn/internal/common"
)

// CachedFileKey ...
type CachedFileKey struct {
	path string
	common.SHA256Struct
}

type cachedFile struct {
	pathInCache string
	fileSize    int64
	lruNode     *lruNode
}

type lruNode struct {
	next, prev *lruNode
	key        CachedFileKey
}

// FileCache ...
type FileCache struct {
	table            map[CachedFileKey]cachedFile
	lruTail, lruHead *lruNode
	mu               sync.Mutex

	uniqueCounter uint64
	cacheDir      string

	totalSizeOnDisk int64
	hardLimit       int64
	softLimit       int64
}

const dirShards = 10

// MakeFileCache ...
func MakeFileCache(cacheDir string, cacheLimitBytes int64) (*FileCache, error) {
	if err := os.MkdirAll(cacheDir, os.ModePerm); err != nil {
		return nil, err
	}
	for i := 0; i < dirShards; i++ {
		dir := path.Join(cacheDir, strconv.Itoa(i))
		if err := os.Mkdir(dir, os.ModePerm); err != nil {
			return nil, err
		}
	}
	return &FileCache{
		table:     make(map[CachedFileKey]cachedFile, 128*1024),
		cacheDir:  cacheDir,
		hardLimit: cacheLimitBytes,
		softLimit: int64(80.0 * (float64(cacheLimitBytes) / 100.0)),
	}, nil
}

// CreateLinkFromCache ...
func (cache *FileCache) CreateLinkFromCache(filePath string, fileSHA256 common.SHA256Struct, destPath string) bool {
	key := CachedFileKey{filePath, fileSHA256}
	cache.mu.Lock()
	cachedFile := cache.table[key]
	if cachedFile.lruNode != nil && cachedFile.lruNode != cache.lruHead {
		// cachedFile.lruNode != cache.lruHead => cachedFile.lruNode.prev != nil
		cachedFile.lruNode.prev.next = cachedFile.lruNode.next
		if cachedFile.lruNode.next == nil {
			// cachedFile.lruNode.next == nil => cachedFile.lruNode == cache.lruTail
			cache.lruTail = cachedFile.lruNode.prev
		} else {
			cachedFile.lruNode.next.prev = cachedFile.lruNode.prev
		}

		cachedFile.lruNode.prev = nil
		cachedFile.lruNode.next = cache.lruHead

		cache.lruHead.prev = cachedFile.lruNode
		cache.lruHead = cachedFile.lruNode
	}
	cache.mu.Unlock()

	if cachedFile.lruNode == nil {
		return false
	}
	if err := os.MkdirAll(path.Dir(destPath), os.ModePerm); err != nil {
		return false
	}
	return os.Link(cachedFile.pathInCache, destPath) == nil
}

// SaveFileToCache ...
func (cache *FileCache) SaveFileToCache(srcPath string, filePath string, fileSHA256 common.SHA256Struct, fileSize int64) (int, bool, error) {
	uniqueID := atomic.AddUint64(&cache.uniqueCounter, 1) - 1
	cachedFileName := fmt.Sprintf("%d/%s.%X", uniqueID%dirShards, path.Base(filePath), uniqueID)
	cachedFilePath := path.Join(cache.cacheDir, cachedFileName)

	if err := os.Link(srcPath, cachedFilePath); err != nil {
		return 0, false, err
	}

	key := CachedFileKey{filePath, fileSHA256}
	newHead := &lruNode{key: key}
	value := cachedFile{pathInCache: cachedFilePath, fileSize: fileSize, lruNode: newHead}
	cache.mu.Lock()
	_, exist := cache.table[key]
	if !exist {
		atomic.AddInt64(&cache.totalSizeOnDisk, fileSize)
		cache.table[key] = value
		newHead.next = cache.lruHead
		if cache.lruHead != nil {
			cache.lruHead.prev = newHead
		}
		cache.lruHead = newHead
		if cache.lruTail == nil {
			cache.lruTail = newHead
		}
	}
	cache.mu.Unlock()

	if exist {
		_ = os.Remove(cachedFilePath)
	}

	return cache.purgeLastElementsTillLimit(cache.hardLimit), !exist, nil
}

// PurgeLastElementsIfRequired ...
func (cache *FileCache) PurgeLastElementsIfRequired() int {
	return cache.purgeLastElementsTillLimit(cache.softLimit)
}

// GetFilesCountAndDiskUsage ...
func (cache *FileCache) GetFilesCountAndDiskUsage() (int64, int64) {
	cache.mu.Lock()
	elements := len(cache.table)
	cache.mu.Unlock()
	return int64(elements), atomic.LoadInt64(&cache.totalSizeOnDisk)
}

func (cache *FileCache) purgeLastElementsTillLimit(cacheLimit int64) int {
	purged := 0

	for atomic.LoadInt64(&cache.totalSizeOnDisk) > cacheLimit {
		var removingFile cachedFile
		cache.mu.Lock()
		if tail := cache.lruTail; tail != nil {
			cache.lruTail = tail.prev
			cache.lruTail.next = nil
			removingFile = cache.table[tail.key]
			delete(cache.table, tail.key)
		}
		cache.mu.Unlock()

		if removingFile.lruNode != nil {
			_ = os.Remove(removingFile.pathInCache)
			atomic.AddInt64(&cache.totalSizeOnDisk, -removingFile.fileSize)
			purged++
		}

	}

	return purged
}
