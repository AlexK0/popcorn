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
}

const POPCORN_SERVER_USER_DIR = "/popcorn-server-user/"

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
		fmt.Fprintf(&keyBuilder, "f:%s/0x%X/0x%X/0x%X/0x%X/", requiredFile.FilePath, requiredFile.B0_7, requiredFile.B8_15, requiredFile.B16_23, requiredFile.B24_31)
		sha256xor.B0_7 ^= requiredFile.B0_7
		sha256xor.B8_15 ^= requiredFile.B8_15
		sha256xor.B16_23 ^= requiredFile.B16_23
		sha256xor.B24_31 ^= requiredFile.B24_31
	}
	return sha256xor, keyBuilder.String()
}

func (session *ClientSession) GetFilePathInWorkingDir(filePathOnClientFileSystem string) (relative string, absolute string) {
	if session.UseObjectCache {
		filePathOnClientFileSystem = strings.Replace(filePathOnClientFileSystem, session.clientUserDir, POPCORN_SERVER_USER_DIR, 1)
	}
	relative = strings.TrimLeft(filePathOnClientFileSystem, "/")
	absolute = path.Join(session.WorkingDir, relative)
	return
}

func (session *ClientSession) GetDirPathInWorkingDir(dirPathOnClientFileSystem string) (relative string, absolute string) {
	if !strings.HasSuffix(dirPathOnClientFileSystem, "/") {
		dirPathOnClientFileSystem += "/"
	}
	return session.GetFilePathInWorkingDir(dirPathOnClientFileSystem)
}

func (session *ClientSession) RemoveUnusedIncludeDirsAndGetCompilerArgs() []string {
	compilerArgs := make([]string, 0, len(session.compilerArgs))
	for i := 0; i < len(session.compilerArgs); i++ {
		arg := session.compilerArgs[i]
		if (arg == "-I" || arg == "-isystem" || arg == "-iquote") && i+1 < len(session.compilerArgs) {
			i++
			includeDirRel, includeDirAbs := session.GetDirPathInWorkingDir(session.compilerArgs[i])
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
		RequiredFilesMeta: make([]requiredFileMetadata, 0, len(in.RequiredFiles)),
		Compiler:          in.Compiler,
		UseObjectCache:    in.UseObjectCache,
		ClientInfo:        clientInfo,
	}
	for _, meta := range in.RequiredFiles {
		fileSHA256, _ := clientInfo.FileSHA256Cache.GetFileSHA256(meta.FilePath, meta.MTime, meta.FileSize)
		newSession.RequiredFilesMeta = append(newSession.RequiredFilesMeta, requiredFileMetadata{
			FileMetadata: meta,
			SHA256Struct: fileSHA256,
		})
	}
	s.mu.Lock()
	sessionID := s.sessionsCounter
	s.sessionsCounter++
	s.sessions[sessionID] = newSession
	s.mu.Unlock()

	newSession.WorkingDir = path.Join(sessionsDir, fmt.Sprint(sessionID))

	inFileRel, _ := newSession.GetFilePathInWorkingDir(in.SourceFilePath)
	outFileRel, outFileAbs := newSession.GetFilePathInWorkingDir(in.SourceFilePath + ".o")

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
	acriveSessions := len(s.sessions)
	s.mu.RUnlock()
	return int64(acriveSessions)
}
