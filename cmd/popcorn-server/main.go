package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"path"
	"syscall"
	"time"

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

	version := flag.Bool("version", false, "Show version and exit.")
	flag.StringVar(&settings.Host, "host", "0.0.0.0", "Binding address.")
	flag.IntVar(&settings.Port, "port", 43210, "Listening port.")
	flag.StringVar(&settings.Password, "password", "", "Secret password for remote control.")
	flag.StringVar(&settings.WorkingDir, "working-dir", "/tmp/popcorn-server", "Directory for saving and compiling incoming files.")
	flag.StringVar(&settings.LogFileName, "log-filename", "", "Logger file.")
	flag.StringVar(&settings.LogSeverity, "log-severity", common.WarningSeverity, "Logger severity level.")

	flag.Parse()

	if *version {
		fmt.Println(common.GetVersion())
		os.Exit(0)
	}

	if !cleanupWorkingDir(settings.WorkingDir) {
		common.LogFatal("Can't create working directory", settings.WorkingDir)
	}

	if err := common.LoggerInit("popcorn-server", settings.LogFileName, settings.LogSeverity); err != nil {
		common.LogFatal("Can't init logger", err)
	}

	addr := fmt.Sprintf("%s:%d", settings.Host, settings.Port)
	common.LogInfo("Start listening", addr)
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		common.LogFatal("Failed to listen:", err)
	}

	grpcServer := grpc.NewServer(grpc.MaxRecvMsgSize(1024*1204*1024), grpc.MaxSendMsgSize(1024*1204*1024))
	compilationServer := &server.CompilationServer{
		StartTime:             time.Now(),
		WorkingDir:            settings.WorkingDir,
		HeaderCacheDir:        path.Join(settings.WorkingDir, "header-cache"),
		GRPCServer:            grpcServer,
		RemoteControlPassword: settings.Password,

		ClientCache:      server.MakeUserCache(),
		UploadingHeaders: server.MakeProcessingHeaders(),
		SystemHeaders:    server.MakeSystemHeaderCache(),
		Sessions:         server.MakeUserSessions(),
	}
	pb.RegisterCompilationServiceServer(grpcServer, compilationServer)
	if err := grpcServer.Serve(lis); err != nil {
		common.LogFatal("Failed to serve:", err)
	}

	grpcServer.Stop()
	lis.Close()

	if len(compilationServer.NewPopcornServerBinaryPath) != 0 {
		if _, err := syscall.ForkExec(compilationServer.NewPopcornServerBinaryPath, os.Args, nil); err != nil {
			common.LogError("Can't restart server:", err)
		}
	}
}
