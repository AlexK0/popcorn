package server

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/AlexK0/popcorn/internal/common"
)

type Client struct {
	FileSHA256Cache
	lastSeen int64
}

type Clients struct {
	table         map[common.SHA256Struct]*Client
	mu            sync.RWMutex
	lastPurgeTime int64
}

func MakeClients() *Clients {
	return &Clients{
		table: make(map[common.SHA256Struct]*Client, 1024),
	}
}

func (clients *Clients) GetClient(userID common.SHA256Struct) *Client {
	clients.mu.RLock()
	client := clients.table[userID]
	clients.mu.RUnlock()

	if client == nil {
		newClient := &Client{FileSHA256Cache: FileSHA256Cache{table: make(map[string]fileMeta, 1024)}}

		clients.mu.Lock()
		client = clients.table[userID]
		if client == nil {
			clients.table[userID] = newClient
			client = newClient
		}
		clients.mu.Unlock()
	}
	atomic.StoreInt64(&client.lastSeen, time.Now().UnixNano())
	return client
}

func (clients *Clients) Count() int64 {
	clients.mu.RLock()
	clientsCount := len(clients.table)
	clients.mu.RUnlock()
	return int64(clientsCount)
}

func (clients *Clients) GetRandomClientCacheSize() int64 {
	clients.mu.RLock()
	defer clients.mu.RUnlock()
	for _, client := range clients.table {
		return client.FileSHA256Cache.Count()
	}
	return 0
}

func (clients *Clients) PurgeOutdatedClients() {
	now := time.Now().UnixNano()
	if time.Duration(now-clients.lastPurgeTime) < time.Hour {
		return
	}

	emptyKey := common.SHA256Struct{}
	outdatedClientKey := emptyKey
	clients.mu.RLock()
	for clientKey, client := range clients.table {
		if time.Duration(now-atomic.LoadInt64(&client.lastSeen)) > 24*time.Hour {
			outdatedClientKey = clientKey
			break
		}
	}
	clients.mu.RUnlock()

	if outdatedClientKey != emptyKey {
		clients.mu.Lock()
		delete(clients.table, outdatedClientKey)
		clients.mu.Unlock()
	} else {
		clients.lastPurgeTime = now
	}
}
