package server

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/AlexK0/popcorn/internal/common"
)

type Client struct {
	HeaderSHA256Cache FileSHA256Cache
	userLastSeen      int64
}

type Clients struct {
	table         map[common.SHA256Struct]*Client
	mu            sync.RWMutex
	lastPurgeTime int64
}

func MakeUsers() *Clients {
	return &Clients{
		table: make(map[common.SHA256Struct]*Client, 1024),
	}
}

func (users *Clients) GetUser(userID common.SHA256Struct) *Client {
	users.mu.RLock()
	user := users.table[userID]
	users.mu.RUnlock()

	if user == nil {
		newUser := &Client{
			HeaderSHA256Cache: FileSHA256Cache{table: make(map[string]fileMeta, 1024)},
		}

		users.mu.Lock()
		user = users.table[userID]
		if user == nil {
			users.table[userID] = newUser
			user = newUser
		}
		users.mu.Unlock()
	}
	atomic.StoreInt64(&user.userLastSeen, time.Now().UnixNano())
	return user
}

func (users *Clients) Count() int64 {
	users.mu.RLock()
	usersCount := len(users.table)
	users.mu.RUnlock()
	return int64(usersCount)
}

func (users *Clients) GetRandomClientCacheSize() int64 {
	users.mu.RLock()
	defer users.mu.RUnlock()
	for _, user := range users.table {
		return user.HeaderSHA256Cache.Count()
	}
	return 0
}

func (users *Clients) PurgeOutdatedUsers() {
	now := time.Now().UnixNano()
	if time.Duration(now-users.lastPurgeTime) < time.Hour {
		return
	}

	emptyKey := common.SHA256Struct{}
	outdatedUserKey := emptyKey
	users.mu.RLock()
	for userKey, user := range users.table {
		if time.Duration(now-atomic.LoadInt64(&user.userLastSeen)) > 24*time.Hour {
			outdatedUserKey = userKey
			break
		}
	}
	users.mu.RUnlock()

	if outdatedUserKey != emptyKey {
		users.mu.Lock()
		delete(users.table, outdatedUserKey)
		users.mu.Unlock()
	} else {
		users.lastPurgeTime = now
	}
}
