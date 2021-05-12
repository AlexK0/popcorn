package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"path"
	"time"

	"github.com/AlexK0/popcorn/internal/common"
	"github.com/AlexK0/popcorn/internal/server"

	"google.golang.org/grpc"

	pb "github.com/AlexK0/popcorn/internal/api/proto/v1"
	_ "google.golang.org/grpc/encoding/gzip" // enables gzip compression for server
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
	flag.StringVar(&settings.WorkingDir, "working-dir", "/tmp/popcorn-server", "Directory for saving and compiling incoming files.")
	flag.StringVar(&settings.LogFileName, "log-filename", "", "Logger file.")
	flag.StringVar(&settings.LogSeverity, "log-severity", common.WarningSeverity, "Logger severity level.")
	flag.Int64Var(&settings.SrcCacheLimit, "src-cache-limit", 512*1024*1024, "Header and source cache limit in bytes.")
	flag.Int64Var(&settings.ObjCacheLimit, "obj-cache-limit", 1024*1024*1024, "Compiled object cache limit in bytes.")
	flag.StringVar(&settings.StatsdAddress, "statsd", "", "Statsd address.")

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

	serverStats, err := server.MakeServerStats(settings.StatsdAddress)
	if err != nil {
		common.LogFatal("Failed to connect to statsd:", err)
	}

	addr := fmt.Sprintf("%s:%d", settings.Host, settings.Port)
	common.LogInfo("Start listening", addr)
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		common.LogFatal("Failed to listen:", err)
	}

	srcCache, err := server.MakeFileCache(path.Join(settings.WorkingDir, "src-cache"), settings.SrcCacheLimit)
	if err != nil {
		common.LogFatal("Failed to init src file cache:", err)
	}

	objCache, err := server.MakeFileCache(path.Join(settings.WorkingDir, "obj-cache"), settings.ObjCacheLimit)
	if err != nil {
		common.LogFatal("Failed to init src file cache:", err)
	}

	grpcServer := grpc.NewServer(grpc.MaxRecvMsgSize(1024*1204*1024), grpc.MaxSendMsgSize(1024*1204*1024))
	compilationServer := &server.CompilationServer{
		StartTime:   time.Now(),
		SessionsDir: path.Join(settings.WorkingDir, "sessions"),
		GRPCServer:  grpcServer,

		RemoteClients:  server.MakeClients(),
		UploadingFiles: server.MakeTransferringFiles(),
		SystemHeaders:  server.MakeSystemHeaderCache(),
		SrcFileCache:   srcCache,
		ObjFileCache:   objCache,

		ActiveSessions: server.MakeSessions(),

		Stats: serverStats,
	}
	pb.RegisterCompilationServiceServer(grpcServer, compilationServer)

	cron := server.Cron{Server: compilationServer}
	cron.Start()

	if err := grpcServer.Serve(lis); err != nil {
		common.LogFatal("Failed to serve:", err)
	}

	cron.Stop()
	grpcServer.Stop()
	serverStats.Close()
	lis.Close()
}
