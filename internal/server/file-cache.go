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
	path     string
	key      common.SHA256Struct
	extraKey common.SHA256Struct
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

	purgedElements int64
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
func (cache *FileCache) CreateLinkFromCacheExtra(filePath string, key common.SHA256Struct, extraKey common.SHA256Struct, destPath string) bool {
	cacheKey := CachedFileKey{path.Base(filePath), key, extraKey}
	cache.mu.Lock()
	cachedFile := cache.table[cacheKey]
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

func (cache *FileCache) CreateLinkFromCache(filePath string, fileSHA256key common.SHA256Struct, destPath string) bool {
	return cache.CreateLinkFromCacheExtra(filePath, fileSHA256key, common.SHA256Struct{}, destPath)
}

// SaveFileToCache ...
func (cache *FileCache) SaveFileToCacheExtra(srcPath string, filePath string, key common.SHA256Struct, extraKey common.SHA256Struct, fileSize int64) (bool, error) {
	uniqueID := atomic.AddUint64(&cache.uniqueCounter, 1) - 1
	fileName := path.Base(filePath)
	cachedFileName := fmt.Sprintf("%d/%s.%X", uniqueID%dirShards, fileName, uniqueID)
	cachedFilePath := path.Join(cache.cacheDir, cachedFileName)

	if err := os.Link(srcPath, cachedFilePath); err != nil {
		return false, err
	}

	cacheKey := CachedFileKey{fileName, key, extraKey}
	newHead := &lruNode{key: cacheKey}
	value := cachedFile{pathInCache: cachedFilePath, fileSize: fileSize, lruNode: newHead}
	cache.mu.Lock()
	_, exists := cache.table[cacheKey]
	if !exists {
		atomic.AddInt64(&cache.totalSizeOnDisk, fileSize)
		cache.table[cacheKey] = value
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

	if exists {
		_ = os.Remove(cachedFilePath)
	}

	cache.purgeLastElementsTillLimit(cache.hardLimit)
	return !exists, nil
}

func (cache *FileCache) SaveFileToCache(srcPath string, filePath string, fileSHA256 common.SHA256Struct, fileSize int64) (bool, error) {
	return cache.SaveFileToCacheExtra(srcPath, filePath, fileSHA256, common.SHA256Struct{}, fileSize)
}

// PurgeLastElementsIfRequired ...
func (cache *FileCache) PurgeLastElementsIfRequired() {
	cache.purgeLastElementsTillLimit(cache.softLimit)
}

// GetFilesCount ...
func (cache *FileCache) GetFilesCount() int64 {
	cache.mu.Lock()
	elements := len(cache.table)
	cache.mu.Unlock()
	return int64(elements)
}

// GetBytesOnDisk ...
func (cache *FileCache) GetBytesOnDisk() int64 {
	return atomic.LoadInt64(&cache.totalSizeOnDisk)
}

// GetPurgedFiles ...
func (cache *FileCache) GetPurgedFiles() int64 {
	return atomic.LoadInt64(&cache.purgedElements)
}

func (cache *FileCache) purgeLastElementsTillLimit(cacheLimit int64) {
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
			atomic.AddInt64(&cache.purgedElements, 1)
		}
	}
}
