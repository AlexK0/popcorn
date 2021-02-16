package server

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"

	pb "github.com/AlexK0/popcorn/internal/api/proto/v1"
	"github.com/AlexK0/popcorn/internal/common"
)

// CompilationServer ...
type CompilationServer struct {
	pb.UnimplementedCompilationServiceServer

	WorkingDir     string
	HeaderCacheDir string
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
		cachedHeaderPath := s.makeCachedHeaderPath(header.GlobalMeta.ClientMeta.FilePath, header.GlobalMeta.SHA256Sum)
		if err := common.WriteFile(cachedHeaderPath, header.HeaderBody); err != nil {
			return nil, err
		}
		linkCreated, err := s.tryLinkHeaderFromCache(sysRoot, header.GlobalMeta.ClientMeta.FilePath, header.GlobalMeta.SHA256Sum)
		if err != nil {
			return nil, err
		}
		if !linkCreated {
			return nil, os.ErrNotExist
		}
		headerCache.SetHeaderSHA256(header.GlobalMeta.ClientMeta.FilePath, header.GlobalMeta.ClientMeta.MTime, header.GlobalMeta.SHA256Sum)
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
	if err := common.WriteFile(inFile, in.SourceBody); err != nil {
		return nil, err
	}

	outFile := inFile + ".o"
	compilerArgs := append([]string{"-isysroot", "."}, in.CompilerArgs...)
	compilerArgs = append(compilerArgs, "-o", outFile, strings.TrimLeft(in.FilePath, "/"))
	compilerProc := exec.Command(in.Compiler, compilerArgs...)
	compilerProc.Dir = sysRootPath
	var compilerStderr, compilerStdout bytes.Buffer
	compilerProc.Stderr = &compilerStderr
	compilerProc.Stdout = &compilerStdout

	_ = compilerProc.Run()
	defer os.Remove(inFile)
	defer os.Remove(outFile)

	var compiledSourceBody []byte
	if compilerProc.ProcessState.ExitCode() == 0 {
		if compiledSource, err := common.ReadFile(outFile); err == nil {
			compiledSourceBody = compiledSource.Bytes()
		} else {
			return nil, err
		}
	}
	return &pb.CompileSourceReply{
		CompilerRetCode: int32(compilerProc.ProcessState.ExitCode()),
		CompiledSource:  compiledSourceBody,
		CompilerStdout:  compilerStdout.Bytes(),
		CompilerStderr:  compilerStderr.Bytes(),
	}, nil
}
