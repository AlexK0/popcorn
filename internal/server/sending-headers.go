package server

import (
	"sync"
	"time"

	"github.com/AlexK0/popcorn/internal/common"
)

type sendingHeaderKey struct {
	path string
	common.SHA256Struct
}

// SendingHeaders ...
type SendingHeaders struct {
	headers map[sendingHeaderKey]time.Time
	mu      sync.Mutex
}

// MakeProcessingHeaders ...
func MakeProcessingHeaders() *SendingHeaders {
	return &SendingHeaders{
		headers: make(map[sendingHeaderKey]time.Time, 1024),
	}
}

// StartHeaderSending ...
func (processingHeaders *SendingHeaders) StartHeaderSending(headerPath string, sha256sum common.SHA256Struct) bool {
	key := sendingHeaderKey{headerPath, sha256sum}
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

// ForceStartHeaderSending ...
func (processingHeaders *SendingHeaders) ForceStartHeaderSending(headerPath string, sha256sum common.SHA256Struct) {
	key := sendingHeaderKey{headerPath, sha256sum}
	now := time.Now()
	processingHeaders.mu.Lock()
	processingHeaders.headers[key] = now
	processingHeaders.mu.Unlock()
}

// FinishHeaderSending ...
func (processingHeaders *SendingHeaders) FinishHeaderSending(headerPath string, sha256sum common.SHA256Struct) {
	key := sendingHeaderKey{headerPath, sha256sum}
	processingHeaders.mu.Lock()
	delete(processingHeaders.headers, key)
	processingHeaders.mu.Unlock()
}
