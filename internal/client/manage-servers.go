package client

import (
	"fmt"
	"io/ioutil"
	"path"
	"strings"
	"time"

	pb "github.com/AlexK0/popcorn/internal/api/proto/v1"
	"github.com/AlexK0/popcorn/internal/common"
)

type checkServerRes struct {
	serverStatus   *pb.StatusReply
	err            error
	serverHostPort string
	processingTime time.Duration
}

func checkServer(serverHostPort string, statusChannel chan<- checkServerRes) {
	start := time.Now()
	grpcClient, err := MakeGRPCClient(serverHostPort)
	if err != nil {
		statusChannel <- checkServerRes{err: err, serverHostPort: serverHostPort}
		return
	}
	defer grpcClient.Clear()

	serverStatus, err := grpcClient.Client.Status(grpcClient.CallContext, &pb.StatusRequest{})
	statusChannel <- checkServerRes{serverStatus: serverStatus, err: err, serverHostPort: serverHostPort, processingTime: time.Since(start)}
}

func prettyBytes(b uint64) string {
	if b > 1024*1024*1024 {
		return fmt.Sprintf("%.2fgb", float64(b)/(1024.0*1024.0*1024.0))
	}
	if b > 1024*1024 {
		return fmt.Sprintf("%.2fmb", float64(b)/(1024.0*1024.0))
	}
	if b > 1024 {
		return fmt.Sprintf("%.2fkb", float64(b)/1024.0)
	}
	return fmt.Sprintf("%db", b)
}

// CheckServers ...
func CheckServers(settings *Settings) {
	statusChannel := make(chan checkServerRes)

	for _, serverHostPort := range settings.Servers {
		go checkServer(serverHostPort, statusChannel)
	}

	for range settings.Servers {
		res := <-statusChannel
		fmt.Printf("Server \033[36m%s\033[0m: ", res.serverHostPort)
		if res.err != nil {
			fmt.Println("\033[31munavailable\033[0m")
			fmt.Println("  Error:", res.err)
		} else {
			fmt.Println("\033[32mok\033[0m")
			fmt.Println("  Server Version:", res.serverStatus.ServerVersion)
			fmt.Println("  Uptime:", time.Duration(res.serverStatus.UptimeNanoseconds))
			fmt.Println("  CPU count:", res.serverStatus.CPUsCount)
			fmt.Println("  Goroutines count:", res.serverStatus.ActiveGoroutinesCount)
			fmt.Println("  Clients count:", res.serverStatus.ClientsCount)
			fmt.Println("  System headers count:", res.serverStatus.SystemHeadersUsedCount)
			fmt.Println("  Cached headers count:", res.serverStatus.CachedHeaderOnDiskCount)
			fmt.Println("  Cached headers bytes:", prettyBytes(res.serverStatus.CachedHeaderOnDiskBytes))
			fmt.Println("  Heap allocates bytes:", prettyBytes(res.serverStatus.HeapAllocBytes))
			fmt.Println("  System allocates bytes:", prettyBytes(res.serverStatus.SystemAllocBytes))
			fmt.Println("  Proceesing time:", res.processingTime)
		}
	}
}

type manageServerRes struct {
	err            error
	serverHostPort string
}

func waitServerResAndPrintStatus(serversCount int, manageChannel <-chan manageServerRes) {
	for i := 0; i != serversCount; i++ {
		res := <-manageChannel
		fmt.Printf("Server \033[36m%s\033[0m: ", res.serverHostPort)
		if res.err != nil {
			fmt.Println("\033[31merror\033[0m")
			fmt.Println("  Error:", res.err)
		} else {
			fmt.Println("\033[32mdone\033[0m")
		}
	}
}

func updateServer(newServerBinaryPath string, password string, serverHostPort string, manageChannel chan<- manageServerRes) {
	grpcClient, err := MakeGRPCClient(serverHostPort)
	if err != nil {
		manageChannel <- manageServerRes{err: err, serverHostPort: serverHostPort}
		return
	}
	defer grpcClient.Clear()

	serverBinary, err := ioutil.ReadFile(newServerBinaryPath)
	if err != nil {
		manageChannel <- manageServerRes{err: err, serverHostPort: serverHostPort}
		return
	}
	_, err = grpcClient.Client.UpdateServer(
		grpcClient.CallContext,
		&pb.UpdateServerRequest{NewBinary: serverBinary, Password: password})
	manageChannel <- manageServerRes{err: err, serverHostPort: serverHostPort}
}

// UpdateServers ...
func UpdateServers(settings *Settings, newServerBinaryPath string, password string) {
	manageChannel := make(chan manageServerRes)
	for _, serverHostPort := range settings.Servers {
		go updateServer(newServerBinaryPath, password, serverHostPort, manageChannel)
	}
	waitServerResAndPrintStatus(len(settings.Servers), manageChannel)
}

func restartServer(password string, serverHostPort string, manageChannel chan<- manageServerRes) {
	grpcClient, err := MakeGRPCClient(serverHostPort)
	if err != nil {
		manageChannel <- manageServerRes{err: err, serverHostPort: serverHostPort}
		return
	}
	defer grpcClient.Clear()

	_, err = grpcClient.Client.RestartServer(
		grpcClient.CallContext,
		&pb.RestartServerRequest{Password: password})
	manageChannel <- manageServerRes{err: err, serverHostPort: serverHostPort}
}

// RestartServers ...
func RestartServers(settings *Settings, password string) {
	manageChannel := make(chan manageServerRes)
	for _, serverHostPort := range settings.Servers {
		go restartServer(password, serverHostPort, manageChannel)
	}
	waitServerResAndPrintStatus(len(settings.Servers), manageChannel)
}

func copyServerLog(logsDir string, password string, serverHostPort string, manageChannel chan<- manageServerRes) {
	grpcClient, err := MakeGRPCClient(serverHostPort)
	if err != nil {
		manageChannel <- manageServerRes{err: err, serverHostPort: serverHostPort}
		return
	}
	defer grpcClient.Clear()
	res, err := grpcClient.Client.DumpServerLog(
		grpcClient.CallContext,
		&pb.DumpServerLogRequest{Password: password})
	if err == nil {
		logFileName := path.Join(logsDir, strings.ReplaceAll(serverHostPort, ":", "_")+".log")
		err = common.WriteFile(logFileName, res.LogData)
	}
	manageChannel <- manageServerRes{err: err, serverHostPort: serverHostPort}
}

// CopyLogsFromServers ...
func CopyLogsFromServers(settings *Settings, logsDir string, password string) {
	manageChannel := make(chan manageServerRes)
	for _, serverHostPort := range settings.Servers {
		go copyServerLog(logsDir, password, serverHostPort, manageChannel)
	}
	waitServerResAndPrintStatus(len(settings.Servers), manageChannel)
}
