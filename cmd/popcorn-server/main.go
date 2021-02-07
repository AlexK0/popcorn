package main

import (
	"flag"
	"net"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/AlexK0/popcorn/internal/common"
	"github.com/AlexK0/popcorn/internal/server"

	"google.golang.org/grpc"
	_ "google.golang.org/grpc/encoding/gzip" // enables gzip compression for server

	pb "github.com/AlexK0/popcorn/internal/api/proto/v1"
)

func cleanupWorkingDir(workingDir string) bool {
	oldWorkingDir := workingDir + ".old"

	if err := os.RemoveAll(oldWorkingDir); err != nil {
		common.LogWarning(err)
	}
	if _, err := os.Stat(workingDir); err == nil {
		if err := os.Rename(workingDir, oldWorkingDir); err != nil {
			common.LogWarning(err)
		}
	}
	if err := os.MkdirAll(workingDir, os.ModePerm); err != nil {
		common.LogError(err)
		return false
	}
	return true
}

func main() {
	settings := &server.Settings{}

	settings.Host = flag.String("host", "0.0.0.0", "Binding address.")
	settings.Port = flag.Int("port", 43210, "Listening port.")
	settings.WorkingDir = flag.String("working-dir", "/tmp/popcorn-server", "Directory for saving and compiling incoming files.")
	settings.LogFileName = flag.String("log-filename", "", "Logger file.")
	settings.LogVerbosity = flag.Int("log-verbosity", 0, "Logger verbosity level.")
	settings.LogSeverity = flag.String("log-severity", common.WarningSeverity, "Logger severity level.")

	flag.Parse()

	if !cleanupWorkingDir(*settings.WorkingDir) {
		common.LogFatal("Can't create working directory", *settings.WorkingDir)
	}

	if err := common.LoggerInit("popcorn-server", *settings.LogFileName, *settings.LogSeverity, *settings.LogVerbosity); err != nil {
		common.LogFatal("Can't init logger", err)
	}

	addr := strings.Join([]string{*settings.Host, strconv.Itoa(*settings.Port)}, ":")

	common.LogInfo("Start listening", addr)
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		common.LogFatal("Failed to listen:", err)
	}

	grpcServer := grpc.NewServer(grpc.MaxRecvMsgSize(1024*1204*1024), grpc.MaxSendMsgSize(1024*1204*1024))
	pb.RegisterCompilationServiceServer(grpcServer, &server.CompilationServer{
		WorkingDir:     *settings.WorkingDir,
		HeaderCacheDir: path.Join(*settings.WorkingDir, "header-cache"),
	})
	if err := grpcServer.Serve(lis); err != nil {
		common.LogFatal("Failed to serve:", err)
	}
}
