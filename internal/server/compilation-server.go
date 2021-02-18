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
	"time"

	pb "github.com/AlexK0/popcorn/internal/api/proto/v1"
	"github.com/AlexK0/popcorn/internal/common"
	"google.golang.org/grpc"
)

// CompilationServer ...
type CompilationServer struct {
	pb.UnimplementedCompilationServiceServer

	WorkingDir     string
	HeaderCacheDir string
	StartTime      time.Time

	NewPopcornServerBinaryPath string
	GRPCServer                 *grpc.Server
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

	headerCache := GetClientHeaderCache(in.ClientID.MachineID, in.ClientID.MacAddress, in.ClientID.UserName)
	for index, header := range in.ClientHeaders {
		headerSHA256 := headerCache.GetHeaderSHA256(header.FilePath, header.MTime)
		if len(headerSHA256) != 0 {
			if err := out.Send(&pb.CopyHeadersFromClientCacheReply{MissedHeaderIndex: int32(index), FullCopyRequired: false}); err != nil {
				return err
			}
			continue
		}
		linkCreated, err := s.tryLinkHeaderFromCache(sysRoot, header.FilePath, headerSHA256)
		if err != nil {
			return err
		}
		if !linkCreated {
			if err := out.Send(&pb.CopyHeadersFromClientCacheReply{MissedHeaderIndex: int32(index), FullCopyRequired: true}); err != nil {
				return err
			}
		}
	}
	return nil
}

// CopyHeadersFromGlobalCache ...
func (s *CompilationServer) CopyHeadersFromGlobalCache(in *pb.CopyHeadersFromGlobalCacheRequest, out pb.CompilationService_CopyHeadersFromGlobalCacheServer) error {
	sysRoot := s.makeSysRoot(in.ClientID)
	headerCache := GetClientHeaderCache(in.ClientID.MachineID, in.ClientID.MacAddress, in.ClientID.UserName)
	for index, header := range in.GlobalHeaders {
		linkCreated, err := s.tryLinkHeaderFromCache(sysRoot, header.ClientMeta.FilePath, header.SHA256Sum)
		if err != nil {
			return err
		}
		if linkCreated {
			headerCache.SetHeaderSHA256(header.ClientMeta.FilePath, header.ClientMeta.MTime, header.SHA256Sum)
		} else {
			if err := out.Send(&pb.CopyHeadersFromGlobalCacheReply{MissedHeaderIndex: int32(index)}); err != nil {
				return err
			}
		}
	}
	return nil
}

// CopyHeaders ...
func (s *CompilationServer) CopyHeaders(ctx context.Context, in *pb.CopyHeadersRequest) (*pb.CopyHeadersReply, error) {
	sysRoot := s.makeSysRoot(in.ClientID)
	headerCache := GetClientHeaderCache(in.ClientID.MachineID, in.ClientID.MacAddress, in.ClientID.UserName)
	for _, header := range in.Headers {
		headerPathInEnv := path.Join(sysRoot, header.GlobalMeta.ClientMeta.FilePath)
		if err := common.WriteFile(headerPathInEnv, header.HeaderBody); err != nil {
			return nil, fmt.Errorf("Can't save copying header: %v", err)
		}
		cachedHeaderPath := s.makeCachedHeaderPath(header.GlobalMeta.ClientMeta.FilePath, header.GlobalMeta.SHA256Sum)
		_ = os.MkdirAll(path.Dir(cachedHeaderPath), os.ModePerm)
		if err := os.Link(headerPathInEnv, cachedHeaderPath); err == nil || os.IsExist(err) {
			headerCache.SetHeaderSHA256(header.GlobalMeta.ClientMeta.FilePath, header.GlobalMeta.ClientMeta.MTime, header.GlobalMeta.SHA256Sum)
		}
	}

	return &pb.CopyHeadersReply{}, nil
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

	common.LogInfo("Launch compiler: ", compilerProc.Args)
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
		ClientsCount:            GetClientCachesCount(),
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
	newServerBinaryPath := path.Join(s.WorkingDir, "new-popcorn-server")
	if err := ioutil.WriteFile(newServerBinaryPath, in.NewBinary, 0777); err != nil {
		return nil, fmt.Errorf("Can't write new server binary: %v", err)
	}
	s.NewPopcornServerBinaryPath = newServerBinaryPath
	go waitAndStop(s.GRPCServer)
	return &pb.UpdateServerReply{}, nil
}
