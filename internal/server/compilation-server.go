package server

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	pb "github.com/AlexK0/popcorn/internal/api/proto/v1"
	"github.com/AlexK0/popcorn/internal/common"
	"google.golang.org/grpc"
)

// CompilationServer ...
type CompilationServer struct {
	pb.UnimplementedCompilationServiceServer

	StartTime time.Time

	WorkingDir     string
	HeaderCacheDir string

	GRPCServer                 *grpc.Server
	UpdatePassword             string
	NewPopcornServerBinaryPath string

	updateLock sync.Mutex

	ClientCache      *ClientCacheMap
	UploadingHeaders *ProcessingHeadersMap
}

func (s *CompilationServer) makeSysRoot(clientID *pb.ClientIdentifier) string {
	return path.Join(s.WorkingDir, clientID.MachineID, clientID.MacAddress, clientID.UserName, strconv.Itoa(int(clientID.Pid)))
}

func (s *CompilationServer) makeCachedHeaderPath(headerPath string, headerSHA256 string) string {
	return strings.Join([]string{path.Join(s.HeaderCacheDir, headerPath), headerSHA256}, ".")
}

func (s *CompilationServer) tryLinkHeaderFromCache(sysRoot string, headerPath string, headerSHA256 string) (bool, error) {
	cachedHeaderPath := s.makeCachedHeaderPath(headerPath, headerSHA256)
	headerPathInEnv := path.Join(sysRoot, headerPath)
	if err := os.MkdirAll(path.Dir(headerPathInEnv), os.ModePerm); err != nil {
		return false, err
	}

	err := os.Link(cachedHeaderPath, headerPathInEnv)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

// CopyHeadersFromClientCache ...
func (s *CompilationServer) CopyHeadersFromClientCache(in *pb.CopyHeadersFromClientCacheRequest, out pb.CompilationService_CopyHeadersFromClientCacheServer) error {
	sysRoot := s.makeSysRoot(in.ClientID)
	if in.ClearEnvironmentBeforeCopy {
		if err := os.RemoveAll(sysRoot); err != nil {
			return err
		}
	}

	type deferredHeader struct {
		sha256sum string
		index     int
	}

	deferredHeadersForCopy := make([]deferredHeader, 0, 16)
	headerCache := s.ClientCache.GetHeaderCache(in.ClientID.MachineID, in.ClientID.MacAddress, in.ClientID.UserName)
	for index, header := range in.ClientHeaders {
		headerSHA256 := headerCache.GetHeaderSHA256(header.FilePath, header.MTime)
		if len(headerSHA256) == 0 {
			if err := out.Send(&pb.CopyHeadersFromClientCacheReply{MissedHeaderIndex: int32(index), FullCopyRequired: false}); err != nil {
				return err
			}
			continue
		}
		linkCreated, err := s.tryLinkHeaderFromCache(sysRoot, header.FilePath, headerSHA256)
		if err != nil {
			return err
		}
		if linkCreated {
			continue
		}
		if s.UploadingHeaders.StartHeaderProcessing(header.FilePath, headerSHA256) {
			if err := out.Send(&pb.CopyHeadersFromClientCacheReply{MissedHeaderIndex: int32(index), FullCopyRequired: true}); err != nil {
				s.UploadingHeaders.FinishHeaderProcessing(header.FilePath, headerSHA256)
				return err
			}
		} else {
			deferredHeadersForCopy = append(deferredHeadersForCopy, deferredHeader{headerSHA256, index})
		}
	}

	attemptsLeft := 10
	for len(deferredHeadersForCopy) != 0 && attemptsLeft >= 0 {
		// TODO Why second?
		time.Sleep(time.Second)
		attemptsLeft--

		newDeferredHeadersForCopy := make([]deferredHeader, 0, len(deferredHeadersForCopy))
		for _, h := range deferredHeadersForCopy {
			header := in.ClientHeaders[h.index]
			linkCreated, err := s.tryLinkHeaderFromCache(sysRoot, header.FilePath, h.sha256sum)
			if err != nil {
				return err
			}
			if !linkCreated {
				newDeferredHeadersForCopy = append(newDeferredHeadersForCopy, h)
			}
		}
		deferredHeadersForCopy = newDeferredHeadersForCopy
	}

	for _, h := range deferredHeadersForCopy {
		header := in.ClientHeaders[h.index]
		linkCreated, err := s.tryLinkHeaderFromCache(sysRoot, header.FilePath, h.sha256sum)
		if err != nil {
			return err
		}
		if linkCreated {
			continue
		}
		s.UploadingHeaders.ForceStartHeaderProcessing(header.FilePath, h.sha256sum)
		if err := out.Send(&pb.CopyHeadersFromClientCacheReply{MissedHeaderIndex: int32(h.index), FullCopyRequired: true}); err != nil {
			s.UploadingHeaders.FinishHeaderProcessing(header.FilePath, h.sha256sum)
			return err
		}
	}

	return nil
}

// CopyHeadersFromGlobalCache ...
func (s *CompilationServer) CopyHeadersFromGlobalCache(in *pb.CopyHeadersFromGlobalCacheRequest, out pb.CompilationService_CopyHeadersFromGlobalCacheServer) error {
	sysRoot := s.makeSysRoot(in.ClientID)
	headerCache := s.ClientCache.GetHeaderCache(in.ClientID.MachineID, in.ClientID.MacAddress, in.ClientID.UserName)
	deferredHeadersForCopy := make([]int, 0, 16)
	for index, header := range in.GlobalHeaders {
		linkCreated, err := s.tryLinkHeaderFromCache(sysRoot, header.ClientMeta.FilePath, header.SHA256Sum)
		if err != nil {
			return err
		}
		if linkCreated {
			headerCache.SetHeaderSHA256(header.ClientMeta.FilePath, header.ClientMeta.MTime, header.SHA256Sum)
		} else {
			if s.UploadingHeaders.StartHeaderProcessing(header.ClientMeta.FilePath, header.SHA256Sum) {
				if err := out.Send(&pb.CopyHeadersFromGlobalCacheReply{MissedHeaderIndex: int32(index)}); err != nil {
					s.UploadingHeaders.FinishHeaderProcessing(header.ClientMeta.FilePath, header.SHA256Sum)
					return err
				}
			} else {
				deferredHeadersForCopy = append(deferredHeadersForCopy, index)
			}
		}
	}

	attemptsLeft := 10
	for len(deferredHeadersForCopy) != 0 && attemptsLeft >= 0 {
		// TODO Why second?
		time.Sleep(time.Second)
		attemptsLeft--

		newDeferredHeadersForCopy := make([]int, 0, len(deferredHeadersForCopy))
		for _, index := range deferredHeadersForCopy {
			header := in.GlobalHeaders[index]
			linkCreated, err := s.tryLinkHeaderFromCache(sysRoot, header.ClientMeta.FilePath, header.SHA256Sum)
			if err != nil {
				return err
			}
			if !linkCreated {
				newDeferredHeadersForCopy = append(newDeferredHeadersForCopy, index)
			}
		}
		deferredHeadersForCopy = newDeferredHeadersForCopy
	}

	for _, index := range deferredHeadersForCopy {
		header := in.GlobalHeaders[index]
		linkCreated, err := s.tryLinkHeaderFromCache(sysRoot, header.ClientMeta.FilePath, header.SHA256Sum)
		if err != nil {
			return err
		}
		if linkCreated {
			headerCache.SetHeaderSHA256(header.ClientMeta.FilePath, header.ClientMeta.MTime, header.SHA256Sum)
			continue
		}
		s.UploadingHeaders.ForceStartHeaderProcessing(header.ClientMeta.FilePath, header.SHA256Sum)
		if err := out.Send(&pb.CopyHeadersFromGlobalCacheReply{MissedHeaderIndex: int32(index)}); err != nil {
			s.UploadingHeaders.FinishHeaderProcessing(header.ClientMeta.FilePath, header.SHA256Sum)
			return err
		}
	}

	return nil
}

// CopyHeader ...
func (s *CompilationServer) CopyHeader(ctx context.Context, in *pb.CopyHeaderRequest) (*pb.CopyHeaderReply, error) {
	defer s.UploadingHeaders.FinishHeaderProcessing(in.Header.GlobalMeta.ClientMeta.FilePath, in.Header.GlobalMeta.SHA256Sum)

	sysRoot := s.makeSysRoot(in.ClientID)
	headerCache := s.ClientCache.GetHeaderCache(in.ClientID.MachineID, in.ClientID.MacAddress, in.ClientID.UserName)
	if linkCreated, _ := s.tryLinkHeaderFromCache(sysRoot, in.Header.GlobalMeta.ClientMeta.FilePath, in.Header.GlobalMeta.SHA256Sum); linkCreated {
		headerCache.SetHeaderSHA256(in.Header.GlobalMeta.ClientMeta.FilePath, in.Header.GlobalMeta.ClientMeta.MTime, in.Header.GlobalMeta.SHA256Sum)
		common.LogInfo("Useless copy, header has been found in cache:", in.Header.GlobalMeta.ClientMeta.FilePath)
		return &pb.CopyHeaderReply{}, nil
	}

	headerPathInEnv := path.Join(sysRoot, in.Header.GlobalMeta.ClientMeta.FilePath)
	if err := common.WriteFile(headerPathInEnv, in.Header.HeaderBody); err != nil {
		return nil, fmt.Errorf("Can't save copying header: %v", err)
	}

	common.LogInfo("Header has been saved on disk:", in.Header.GlobalMeta.ClientMeta.FilePath)
	cachedHeaderPath := s.makeCachedHeaderPath(in.Header.GlobalMeta.ClientMeta.FilePath, in.Header.GlobalMeta.SHA256Sum)
	_ = os.MkdirAll(path.Dir(cachedHeaderPath), os.ModePerm)
	if err := os.Link(headerPathInEnv, cachedHeaderPath); err == nil || os.IsExist(err) {
		headerCache.SetHeaderSHA256(in.Header.GlobalMeta.ClientMeta.FilePath, in.Header.GlobalMeta.ClientMeta.MTime, in.Header.GlobalMeta.SHA256Sum)
	}

	return &pb.CopyHeaderReply{}, nil
}

func removeDir(dirPath string, remove bool) {
	if remove {
		_ = os.RemoveAll(dirPath)
	}
}

// CompileSource ....
func (s *CompilationServer) CompileSource(ctx context.Context, in *pb.CompileSourceRequest) (*pb.CompileSourceReply, error) {
	sysRootPath := s.makeSysRoot(in.ClientID)

	defer removeDir(sysRootPath, in.ClearEnvironmentAfterBuild)

	inFile := path.Join(sysRootPath, in.FilePath)
	err := common.WriteFile(inFile, in.SourceBody)
	if err != nil {
		return nil, fmt.Errorf("Can't write source for compilation: %v", err)
	}

	outFile := inFile + ".o"
	compilerArgs := make([]string, 0, 7+len(in.CompilerArgs))
	compilerArgs = append(compilerArgs, "-isysroot", ".", "-o", outFile, strings.TrimLeft(in.FilePath, "/"))
	compilerArgs = append(compilerArgs, in.CompilerArgs...)
	compilerProc := exec.Command(in.Compiler, compilerArgs...)
	compilerProc.Dir = sysRootPath
	var compilerStderr, compilerStdout bytes.Buffer
	compilerProc.Stderr = &compilerStderr
	compilerProc.Stdout = &compilerStdout

	common.LogInfo("Launch compiler:", compilerProc.Args)
	_ = compilerProc.Run()
	defer os.Remove(inFile)
	defer os.Remove(outFile)

	var compiledSource []byte
	if compilerProc.ProcessState.ExitCode() == 0 {
		if compiledSource, err = ioutil.ReadFile(outFile); err != nil {
			return nil, fmt.Errorf("Can't read compiled source: %v", err)
		}
	}
	return &pb.CompileSourceReply{
		CompilerRetCode: int32(compilerProc.ProcessState.ExitCode()),
		CompiledSource:  compiledSource,
		CompilerStdout:  compilerStdout.Bytes(),
		CompilerStderr:  compilerStderr.Bytes(),
	}, nil
}

// Status ...
func (s *CompilationServer) Status(ctx context.Context, in *pb.StatusRequest) (*pb.StatusReply, error) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	headersCount, headersSize, _ := common.DirElementsAndSize(s.HeaderCacheDir)
	return &pb.StatusReply{
		ServerVersion:           common.GetVersion(),
		CPUsCount:               uint64(runtime.NumCPU()),
		ActiveGoroutinesCount:   uint64(runtime.NumGoroutine()),
		ClientsCount:            s.ClientCache.GetCachesCount(),
		CachedHeaderOnDiskCount: headersCount,
		CachedHeaderOnDiskBytes: headersSize,
		HeapAllocBytes:          m.HeapAlloc,
		SystemAllocBytes:        m.Sys,
		UptimeNanoseconds:       uint64(time.Since(s.StartTime).Nanoseconds()),
	}, nil
}

func waitAndStop(grpcServer *grpc.Server) {
	time.Sleep(time.Millisecond * 100)
	grpcServer.GracefulStop()
}

// UpdateServer ...
func (s *CompilationServer) UpdateServer(ctx context.Context, in *pb.UpdateServerRequest) (*pb.UpdateServerReply, error) {
	s.updateLock.Lock()
	defer s.updateLock.Unlock()

	if len(s.UpdatePassword) == 0 {
		return nil, fmt.Errorf("Remote update disabled")
	}
	if s.UpdatePassword != in.Password {
		s.UpdatePassword = ""
		return nil, fmt.Errorf("Invalid password, disabling remote updates")
	}

	newServerBinaryPath := path.Join(s.WorkingDir, "new-popcorn-server")
	if err := ioutil.WriteFile(newServerBinaryPath, in.NewBinary, 0777); err != nil {
		return nil, fmt.Errorf("Can't write new server binary: %v", err)
	}
	s.NewPopcornServerBinaryPath = newServerBinaryPath
	go waitAndStop(s.GRPCServer)
	return &pb.UpdateServerReply{}, nil
}
