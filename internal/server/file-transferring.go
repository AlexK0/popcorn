package server

import (
	"path/filepath"
	"sync"
	"time"

	"github.com/AlexK0/popcorn/internal/common"
)

type transferringFileKey struct {
	path string
	common.SHA256Struct
}

type FileTransferring struct {
	files map[transferringFileKey]time.Time
	mu    sync.Mutex
}

func MakeTransferringFiles() *FileTransferring {
	return &FileTransferring{
		files: make(map[transferringFileKey]time.Time, 1024),
	}
}

func (transferring *FileTransferring) StartFileTransfer(filePath string, filesha256 common.SHA256Struct) bool {
	key := transferringFileKey{filepath.Base(filePath), filesha256}
	now := time.Now()
	started := false
	transferring.mu.Lock()
	processingStartTime, alreadyStarted := transferring.files[key]
	// TODO Why 5 second?
	if !alreadyStarted || now.Sub(processingStartTime) > time.Second*5 {
		transferring.files[key] = now
		started = true
	}
	transferring.mu.Unlock()
	return started
}

func (transferring *FileTransferring) ForceStartFileTransfer(filePath string, filesha256 common.SHA256Struct) {
	key := transferringFileKey{filepath.Base(filePath), filesha256}
	now := time.Now()
	transferring.mu.Lock()
	transferring.files[key] = now
	transferring.mu.Unlock()
}

func (transferring *FileTransferring) FinishFileTransfer(filePath string, filesha256 common.SHA256Struct) {
	key := transferringFileKey{filepath.Base(filePath), filesha256}
	transferring.mu.Lock()
	delete(transferring.files, key)
	transferring.mu.Unlock()
}

func (transferring *FileTransferring) TransferringFilesCount() int64 {
	transferring.mu.Lock()
	count := len(transferring.files)
	transferring.mu.Unlock()
	return int64(count)
}
