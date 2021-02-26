package server

import (
	"sync"

	"github.com/AlexK0/popcorn/internal/common"

	pb "github.com/AlexK0/popcorn/internal/api/proto/v1"
)

// RequiredHeaderMetadata ...
type RequiredHeaderMetadata struct {
	*pb.HeaderMetadata
	common.SHA256Struct
}

// UserSession ...
type UserSession struct {
	UserID common.SHA256Struct

	SourceFilePath string
	Compiler       string
	CompilerArgs   []string
	WorkingDir     string

	RequiredHeaders []RequiredHeaderMetadata

	FileSHA256Cache *FileSHA256Cache
}

// UserSessions ...
type UserSessions struct {
	sessions map[uint64]*UserSession

	sessionsCounter uint64
	mu              sync.RWMutex
}

// MakeUserSessions ...
func MakeUserSessions() *UserSessions {
	return &UserSessions{
		sessions: make(map[uint64]*UserSession, 512),
	}
}

// OpenNewSession ...
func (s *UserSessions) OpenNewSession(newSession *UserSession) uint64 {
	s.mu.Lock()
	sessionID := s.sessionsCounter
	s.sessionsCounter++
	s.sessions[sessionID] = newSession
	s.mu.Unlock()
	return sessionID
}

// GetSession ...
func (s *UserSessions) GetSession(sessionID uint64) *UserSession {
	s.mu.RLock()
	session := s.sessions[sessionID]
	s.mu.RUnlock()
	return session
}

// CloseSession ...
func (s *UserSessions) CloseSession(sessionID uint64) {
	s.mu.Lock()
	delete(s.sessions, sessionID)
	s.mu.Unlock()
}

// ActiveSessions ...
func (s *UserSessions) ActiveSessions() int64 {
	s.mu.RLock()
	acriveSessions := len(s.sessions)
	s.mu.RUnlock()
	return int64(acriveSessions)
}
