package client

import (
	"fmt"
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
			fmt.Println("  CPU count:", res.serverStatus.CPUsCount)
			fmt.Println("  Goroutines count:", res.serverStatus.ActiveGoroutinesCount)
			fmt.Println("  Clients count:", res.serverStatus.ClientsCount)
			fmt.Println("  Cached headers count:", res.serverStatus.CachedHeaderOnDiskCount)
			fmt.Println("  Cached headers bytes:", prettyBytes(res.serverStatus.CachedHeaderOnDiskBytes))
			fmt.Println("  Heap allocates bytes:", prettyBytes(res.serverStatus.HeapAllocBytes))
			fmt.Println("  System allocates bytes:", prettyBytes(res.serverStatus.SystemAllocBytes))
			fmt.Println("  Proceesing time:", res.processingTime)
		}
	}
}
