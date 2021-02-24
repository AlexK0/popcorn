package server

import (
	"fmt"
	"os"
	"path"
	"strconv"
	"sync"

	"github.com/AlexK0/popcorn/internal/common"
)

// CachedFileKey ...
type CachedFileKey struct {
	path string
	common.SHA256Struct
}

// FileCache ...
type FileCache struct {
	table map[CachedFileKey]string
	mu    sync.Locker

	cacheDir string
}

const dirShards = 10

// MakeFileCache ...
func MakeFileCache(cacheDir string) (*FileCache, error) {
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
		table:    make(map[CachedFileKey]string, 128*1024),
		cacheDir: cacheDir,
	}, nil
}

// CreateLinkFromCache ...
func (cache *FileCache) CreateLinkFromCache(filePath string, fileSHA256 common.SHA256Struct, destPath string) bool {
	cache.mu.Lock()
	cachedFilePath := cache.table[CachedFileKey{filePath, fileSHA256}]
	cache.mu.Unlock()

	if len(cachedFilePath) == 0 {
		return false
	}
	if err := os.MkdirAll(path.Dir(destPath), os.ModePerm); err != nil {
		return false
	}
	return os.Link(cachedFilePath, destPath) == nil
}

// SaveFileToCache ...
func (cache *FileCache) SaveFileToCache(srcPath string, filePath string, fileSHA256 common.SHA256Struct) error {
	shardID := (fileSHA256.B0_7 ^ fileSHA256.B8_15 ^ fileSHA256.B16_23 ^ fileSHA256.B24_31) % dirShards
	cachedFileName := fmt.Sprintf("%d/%s.%X.%X.%X.%X", shardID, path.Base(filePath), fileSHA256.B0_7, fileSHA256.B8_15, fileSHA256.B16_23, fileSHA256.B24_31)
	cachedFilePath := path.Join(cache.cacheDir, cachedFileName)

	if err := os.Link(srcPath, cachedFilePath); err != nil {
		if os.IsExist(err) {
			return nil
		}
		return err
	}

	cache.mu.Lock()
	cache.table[CachedFileKey{filePath, fileSHA256}] = cachedFilePath
	cache.mu.Unlock()
	return nil
}
