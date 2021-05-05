package server

import (
	"sync"

	"github.com/AlexK0/popcorn/internal/common"
)

type User struct {
	HeaderSHA256Cache FileSHA256Cache
}

type Users struct {
	users map[common.SHA256Struct]*User
	mu    sync.RWMutex
}

func MakeUsers() *Users {
	return &Users{
		users: make(map[common.SHA256Struct]*User, 1024),
	}
}

func (users *Users) GetUser(userID common.SHA256Struct) *User {
	users.mu.RLock()
	user := users.users[userID]
	users.mu.RUnlock()

	if user != nil {
		return user
	}

	newUser := &User{
		HeaderSHA256Cache: FileSHA256Cache{table: make(map[string]fileMeta, 1024)},
	}

	users.mu.Lock()
	user = users.users[userID]
	if user == nil {
		users.users[userID] = newUser
		user = newUser
	}
	users.mu.Unlock()
	return user
}

func (users *Users) GetUsersCount() int64 {
	users.mu.RLock()
	usersCount := len(users.users)
	users.mu.RUnlock()
	return int64(usersCount)
}
