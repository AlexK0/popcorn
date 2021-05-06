package server

import (
	"fmt"
	"path"
	"strings"
	"sync"

	"github.com/AlexK0/popcorn/internal/common"

	pb "github.com/AlexK0/popcorn/internal/api/proto/v1"
)

// RequiredHeaderMetadata ...
type RequiredHeaderMetadata struct {
	*pb.HeaderMetadata
	common.SHA256Struct
	UseFromSystem bool
}

const POPCORN_SERVER_USER_DIR = "/popcorn-server-user/"

// UserSession ...
type UserSession struct {
	userDir      string
	compilerArgs []string

	SourceFilePath    string
	OutObjectFilePath string
	Compiler          string
	WorkingDir        string
	UseObjectCache    bool

	RequiredHeaders []RequiredHeaderMetadata

	UserInfo *Client
}

func (session *UserSession) GetFilePathInWorkingDir(filePathOnClientFileSystem string) (relative string, absolute string) {
	if session.UseObjectCache {
		filePathOnClientFileSystem = strings.Replace(filePathOnClientFileSystem, session.userDir, POPCORN_SERVER_USER_DIR, 1)
	}
	relative = strings.TrimLeft(filePathOnClientFileSystem, "/")
	absolute = path.Join(session.WorkingDir, filePathOnClientFileSystem)
	return
}

func (session *UserSession) GetDirPathInWorkingDir(dirPathOnClientFileSystem string) (relative string, absolute string) {
	if !strings.HasSuffix(dirPathOnClientFileSystem, "/") {
		dirPathOnClientFileSystem += "/"
	}
	return session.GetFilePathInWorkingDir(dirPathOnClientFileSystem)
}

func (session *UserSession) RemoveUnusedIncludeDirsAndGetCompilerArgs() []string {
	compilerArgs := make([]string, 0, len(session.compilerArgs))
	for i := 0; i < len(session.compilerArgs); i++ {
		arg := session.compilerArgs[i]
		if (arg == "-I" || arg == "-isystem" || arg == "-iquote") && i+1 < len(session.compilerArgs) {
			i++
			includeDir := session.compilerArgs[i]
			dirIsUsed := func() bool {
				// TODO write something better?
				for _, usedHeader := range session.RequiredHeaders {
					if !usedHeader.UseFromSystem && strings.HasPrefix(usedHeader.FilePath, includeDir) {
						return true
					}
				}
				return false
			}()

			if dirIsUsed {
				includeDir, _ = session.GetDirPathInWorkingDir(includeDir)
				compilerArgs = append(compilerArgs, arg)
				compilerArgs = append(compilerArgs, includeDir)
			}
			continue
		}
		compilerArgs = append(compilerArgs, arg)
	}
	return compilerArgs
}

// Sessions ...
type Sessions struct {
	sessions map[uint64]*UserSession

	sessionsCounter uint64
	mu              sync.RWMutex
}

// MakeUserSessions ...
func MakeUserSessions() *Sessions {
	return &Sessions{
		sessions: make(map[uint64]*UserSession, 512),
	}
}

// OpenNewSession ...
func (s *Sessions) OpenNewSession(in *pb.StartCompilationSessionRequest, sessionsDir string, userInfo *Client) (uint64, *UserSession) {
	newSession := &UserSession{
		userDir:         "/" + in.UserName + "/",
		Compiler:        in.Compiler,
		UseObjectCache:  in.UseObjectCache,
		RequiredHeaders: make([]RequiredHeaderMetadata, 0, len(in.RequiredHeaders)),
		UserInfo:        userInfo,
	}
	for _, headerMetadata := range in.RequiredHeaders {
		headerSHA256, _ := userInfo.HeaderSHA256Cache.GetFileSHA256(headerMetadata.FilePath, headerMetadata.MTime)
		newSession.RequiredHeaders = append(newSession.RequiredHeaders, RequiredHeaderMetadata{
			HeaderMetadata: headerMetadata,
			SHA256Struct:   headerSHA256,
		})
	}
	s.mu.Lock()
	sessionID := s.sessionsCounter
	s.sessionsCounter++
	s.sessions[sessionID] = newSession
	s.mu.Unlock()

	newSession.WorkingDir = path.Join(sessionsDir, fmt.Sprint(sessionID))

	inFileRel, inFileAbs := newSession.GetFilePathInWorkingDir(in.SourceFilePath)
	outFileRel, outFileAbs := newSession.GetFilePathInWorkingDir(in.SourceFilePath + ".o")

	newSession.SourceFilePath = inFileAbs
	newSession.OutObjectFilePath = outFileAbs
	newSession.compilerArgs = append(in.CompilerArgs, inFileRel, "-o", outFileRel)
	return sessionID, newSession
}

// GetSession ...
func (s *Sessions) GetSession(sessionID uint64) *UserSession {
	s.mu.RLock()
	session := s.sessions[sessionID]
	s.mu.RUnlock()
	return session
}

// CloseSession ...
func (s *Sessions) CloseSession(sessionID uint64) {
	s.mu.Lock()
	delete(s.sessions, sessionID)
	s.mu.Unlock()
}

// ActiveSessions ...
func (s *Sessions) ActiveSessions() int64 {
	s.mu.RLock()
	acriveSessions := len(s.sessions)
	s.mu.RUnlock()
	return int64(acriveSessions)
}
