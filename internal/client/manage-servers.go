package client

import (
	"fmt"
	"strings"
	"time"

	pb "github.com/AlexK0/popcorn/internal/api/proto/v1"
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
