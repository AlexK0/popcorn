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

// CheckServers ...
func CheckServers(settings *Settings, showStats bool) {
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
			fmt.Println("  Server version:", res.serverStatus.ServerVersion)
			fmt.Println("  Server args:", res.serverStatus.ServerArgs)
			fmt.Println("  Proceesing time:", res.processingTime)
			if showStats {
				fmt.Println("  Server stats:")
				statsWithIndent := "    " + strings.TrimSpace(strings.ReplaceAll(string(res.serverStatus.ServerStats), "|g\n", "\n    "))
				fmt.Println(strings.ReplaceAll(statsWithIndent, ":", ": "))
			}
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
