package server

import (
	"fmt"
	"os"
	"path"
	"strings"
	"sync"

	"github.com/AlexK0/popcorn/internal/common"

	pb "github.com/AlexK0/popcorn/internal/api/proto/v1"
)

type requiredFileMetadata struct {
	*pb.FileMetadata
	common.SHA256Struct
	AbsPathInWorkingDir string
	relPathInWorkingDir string
}

type ClientSession struct {
	clientUserDir string
	compilerArgs  []string

	OutObjectFilePath string
	Compiler          string
	WorkingDir        string
	UseObjectCache    bool

	ClientInfo        *Client
	RequiredFilesMeta []requiredFileMetadata
}

func (session *ClientSession) MakeObjectCacheKey() (common.SHA256Struct, string) {
	sha256xor := common.SHA256Struct{}

	keyBuilder := strings.Builder{}
	keyBuilder.Grow(1024)

	keyBuilder.WriteString("compiler-")
	keyBuilder.WriteString(session.Compiler)
	keyBuilder.WriteString(";args-")

	for _, arg := range session.compilerArgs {
		keyBuilder.WriteString(arg)
		keyBuilder.WriteString(" ")
	}

	keyBuilder.WriteString(";depends-")
	for _, requiredFile := range session.RequiredFilesMeta {
		keyBuilder.WriteString("f:")
		keyBuilder.WriteString(requiredFile.relPathInWorkingDir)
		fmt.Fprintf(&keyBuilder, "{0x%X/0x%X/0x%X/0x%X}", requiredFile.B0_7, requiredFile.B8_15, requiredFile.B16_23, requiredFile.B24_31)
		sha256xor.B0_7 ^= requiredFile.B0_7
		sha256xor.B8_15 ^= requiredFile.B8_15
		sha256xor.B16_23 ^= requiredFile.B16_23
		sha256xor.B24_31 ^= requiredFile.B24_31
	}
	return sha256xor, keyBuilder.String()
}

func (session *ClientSession) getPathInWorkingDir(filePathOnClientFileSystem string) (relative string, absolute string) {
	const POPCORN_SERVER_USER_DIR = "/popcorn-server-user/"
	if session.UseObjectCache {
		filePathOnClientFileSystem = strings.Replace(filePathOnClientFileSystem, session.clientUserDir, POPCORN_SERVER_USER_DIR, 1)
	}
	relative = strings.TrimLeft(filePathOnClientFileSystem, "/")
	absolute = path.Join(session.WorkingDir, relative)
	return
}

func (session *ClientSession) RemoveUnusedIncludeDirsAndGetCompilerArgs() []string {
	compilerArgs := make([]string, 0, len(session.compilerArgs))
	for i := 0; i < len(session.compilerArgs); i++ {
		arg := session.compilerArgs[i]
		if (arg == "-I" || arg == "-isystem" || arg == "-iquote") && i+1 < len(session.compilerArgs) {
			i++
			includeDirRel, includeDirAbs := session.getPathInWorkingDir(session.compilerArgs[i])
			if _, err := os.Stat(includeDirAbs); !os.IsNotExist(err) {
				compilerArgs = append(compilerArgs, arg, includeDirRel)
			}
			continue
		}
		compilerArgs = append(compilerArgs, arg)
	}
	return compilerArgs
}

type Sessions struct {
	sessions map[uint64]*ClientSession

	sessionsCounter uint64
	mu              sync.RWMutex
}

func MakeSessions() *Sessions {
	return &Sessions{
		sessions: make(map[uint64]*ClientSession, 1024),
	}
}

func (s *Sessions) OpenNewSession(in *pb.StartCompilationSessionRequest, sessionsDir string, clientInfo *Client) (uint64, *ClientSession) {
	newSession := &ClientSession{
		clientUserDir:     "/" + in.ClientUserName + "/",
		RequiredFilesMeta: make([]requiredFileMetadata, len(in.RequiredFiles)),
		Compiler:          in.Compiler,
		UseObjectCache:    in.UseObjectCache,
		ClientInfo:        clientInfo,
	}

	s.mu.Lock()
	sessionID := s.sessionsCounter
	s.sessionsCounter++
	s.sessions[sessionID] = newSession
	s.mu.Unlock()

	newSession.WorkingDir = path.Join(sessionsDir, fmt.Sprint(sessionID))

	for index, meta := range in.RequiredFiles {
		fileMetadata := &newSession.RequiredFilesMeta[index]
		fileMetadata.FileMetadata = meta
		fileMetadata.SHA256Struct, _ = clientInfo.FileSHA256Cache.GetFileSHA256(meta.FilePath, meta.MTime, meta.FileSize)
		fileMetadata.relPathInWorkingDir, fileMetadata.AbsPathInWorkingDir = newSession.getPathInWorkingDir(meta.FilePath)
	}

	inFileRel, _ := newSession.getPathInWorkingDir(in.SourceFilePath)
	outFileRel, outFileAbs := newSession.getPathInWorkingDir(in.SourceFilePath + ".o")

	newSession.OutObjectFilePath = outFileAbs
	newSession.compilerArgs = append(in.CompilerArgs, inFileRel, "-o", outFileRel)
	return sessionID, newSession
}

func (s *Sessions) GetSession(sessionID uint64) *ClientSession {
	s.mu.RLock()
	session := s.sessions[sessionID]
	s.mu.RUnlock()
	return session
}

func (s *Sessions) CloseSession(sessionID uint64) {
	s.mu.Lock()
	delete(s.sessions, sessionID)
	s.mu.Unlock()
}

func (s *Sessions) ActiveSessions() int64 {
	s.mu.RLock()
	activeSessions := len(s.sessions)
	s.mu.RUnlock()
	return int64(activeSessions)
}
