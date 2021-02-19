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
	SystemHeaders    *SystemHeaderCache
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

type deferredHeader struct {
	headerPath string
	sha256sum  string
	index      int
}

func (s *CompilationServer) waitDeferredHeaders(sysRoot string, headers []deferredHeader) (done []deferredHeader, stillDeferted []deferredHeader, err error) {
	if len(headers) == 0 {
		return nil, nil, nil
	}

	done = make([]deferredHeader, 0, len(headers))

	start := time.Now()
	// TODO Why 6 seconds?
	for len(headers) != 0 && time.Since(start) < 6*time.Second {
		// TODO Why 100 milliseconds?
		time.Sleep(100 * time.Millisecond)
		stillDeferted = make([]deferredHeader, 0, len(headers)-len(done))
		for _, h := range headers {
			linkCreated, err := s.tryLinkHeaderFromCache(sysRoot, h.headerPath, h.sha256sum)
			if err != nil {
				return nil, nil, err
			}
			if linkCreated {
				done = append(done, h)
			} else {
				stillDeferted = append(stillDeferted, h)
			}
		}
		headers = stillDeferted
	}

	return
}

// CopyHeadersFromClientCache ...
func (s *CompilationServer) CopyHeadersFromClientCache(in *pb.CopyHeadersFromClientCacheRequest, out pb.CompilationService_CopyHeadersFromClientCacheServer) error {
	sysRoot := s.makeSysRoot(in.ClientID)
	if in.ClearEnvironmentBeforeCopy {
		if err := os.RemoveAll(sysRoot); err != nil {
			return err
		}
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

		if systemSHA256 := s.SystemHeaders.GetSystemHeaderSHA256(header.FilePath); systemSHA256 == headerSHA256 {
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
			deferredHeadersForCopy = append(deferredHeadersForCopy, deferredHeader{header.FilePath, headerSHA256, index})
		}
	}

	_, stillDeferred, err := s.waitDeferredHeaders(sysRoot, deferredHeadersForCopy)
	if err != nil {
		return err
	}

	for _, h := range stillDeferred {
		s.UploadingHeaders.ForceStartHeaderProcessing(h.headerPath, h.sha256sum)
		if err := out.Send(&pb.CopyHeadersFromClientCacheReply{MissedHeaderIndex: int32(h.index), FullCopyRequired: true}); err != nil {
			s.UploadingHeaders.FinishHeaderProcessing(h.headerPath, h.sha256sum)
			return err
		}
	}

	return nil
}

// CopyHeadersFromGlobalCache ...
func (s *CompilationServer) CopyHeadersFromGlobalCache(in *pb.CopyHeadersFromGlobalCacheRequest, out pb.CompilationService_CopyHeadersFromGlobalCacheServer) error {
	sysRoot := s.makeSysRoot(in.ClientID)
	headerCache := s.ClientCache.GetHeaderCache(in.ClientID.MachineID, in.ClientID.MacAddress, in.ClientID.UserName)
	deferredHeadersForCopy := make([]deferredHeader, 0, 16)
	for index, header := range in.GlobalHeaders {
		if systemSHA256 := s.SystemHeaders.GetSystemHeaderSHA256(header.ClientMeta.FilePath); systemSHA256 == header.SHA256Sum {
			headerCache.SetHeaderSHA256(header.ClientMeta.FilePath, header.ClientMeta.MTime, header.SHA256Sum)
			continue
		}
		linkCreated, err := s.tryLinkHeaderFromCache(sysRoot, header.ClientMeta.FilePath, header.SHA256Sum)
		if err != nil {
			return err
		}
		if linkCreated {
			headerCache.SetHeaderSHA256(header.ClientMeta.FilePath, header.ClientMeta.MTime, header.SHA256Sum)
			continue
		}
		if s.UploadingHeaders.StartHeaderProcessing(header.ClientMeta.FilePath, header.SHA256Sum) {
			if err := out.Send(&pb.CopyHeadersFromGlobalCacheReply{MissedHeaderIndex: int32(index)}); err != nil {
				s.UploadingHeaders.FinishHeaderProcessing(header.ClientMeta.FilePath, header.SHA256Sum)
				return err
			}
		} else {
			deferredHeadersForCopy = append(deferredHeadersForCopy, deferredHeader{header.ClientMeta.FilePath, header.SHA256Sum, index})
		}
	}

	done, stillDeferred, err := s.waitDeferredHeaders(sysRoot, deferredHeadersForCopy)
	if err != nil {
		return err
	}
	for _, h := range done {
		headerCache.SetHeaderSHA256(h.headerPath, in.GlobalHeaders[h.index].ClientMeta.MTime, h.sha256sum)
	}
	for _, h := range stillDeferred {
		s.UploadingHeaders.ForceStartHeaderProcessing(h.headerPath, h.sha256sum)
		if err := out.Send(&pb.CopyHeadersFromGlobalCacheReply{MissedHeaderIndex: int32(h.index)}); err != nil {
			s.UploadingHeaders.FinishHeaderProcessing(h.headerPath, h.sha256sum)
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

	inFileFull := path.Join(sysRootPath, in.FilePath)
	err := common.WriteFile(inFileFull, in.SourceBody)
	if err != nil {
		return nil, fmt.Errorf("Can't write source for compilation: %v", err)
	}

	compilerArgs := make([]string, 0, 3+len(in.CompilerArgs))
	for i := 0; i < len(in.CompilerArgs); i++ {
		arg := in.CompilerArgs[i]
		if (arg == "-I" || arg == "-isystem" || arg == "-iquote") && i+1 < len(in.CompilerArgs) {
			includeDir := in.CompilerArgs[i+1]
			includeDirFull := path.Join(sysRootPath, includeDir)
			if _, err = os.Stat(includeDirFull); err != nil && os.IsNotExist(err) {
				i++
				continue
			}
			in.CompilerArgs[i+1] = strings.TrimLeft(includeDir, "/")
		}
		compilerArgs = append(compilerArgs, arg)
	}
	inFile := strings.TrimLeft(in.FilePath, "/")
	outFile := inFile + ".o"
	compilerArgs = append(compilerArgs, inFile, "-o", outFile)

	compilerProc := exec.Command(in.Compiler, compilerArgs...)
	compilerProc.Dir = sysRootPath
	var compilerStderr, compilerStdout bytes.Buffer
	compilerProc.Stderr = &compilerStderr
	compilerProc.Stdout = &compilerStdout

	common.LogInfo("Launch compiler:", compilerProc.Args)
	_ = compilerProc.Run()

	outFileFull := path.Join(sysRootPath, outFile)
	defer os.Remove(outFileFull)

	var compiledSource []byte
	if compilerProc.ProcessState.ExitCode() == 0 {
		if compiledSource, err = ioutil.ReadFile(outFileFull); err != nil {
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
		SystemHeadersUsedCount:  s.SystemHeaders.GetSystemHeadersCacheSize(),
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
