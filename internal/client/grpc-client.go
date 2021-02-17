package client

import (
	"context"
	"time"

	pb "github.com/AlexK0/popcorn/internal/api/proto/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/encoding/gzip"
)

// GRPCClient ...
type GRPCClient struct {
	Connection  *grpc.ClientConn
	CallContext context.Context
	CancelFunc  context.CancelFunc
	Client      pb.CompilationServiceClient
}

// MakeGRPCClient ...
func MakeGRPCClient(serverHostPort string) (*GRPCClient, error) {
	connection, err := grpc.Dial(serverHostPort,
		grpc.WithInsecure(),
		grpc.WithBlock(),
		grpc.WithDefaultCallOptions(
			grpc.UseCompressor(gzip.Name),
			grpc.MaxCallRecvMsgSize(1024*1204*1024),
			grpc.MaxCallSendMsgSize(1024*1204*1024)))
	if err != nil {
		return nil, err
	}

	ctx, cancelFunc := context.WithTimeout(context.Background(), time.Minute*10)
	return &GRPCClient{
		Connection:  connection,
		CallContext: ctx,
		CancelFunc:  cancelFunc,
		Client:      pb.NewCompilationServiceClient(connection),
	}, nil
}

// Clear ...
func (grpcClient *GRPCClient) Clear() {
	if grpcClient.Connection != nil {
		grpcClient.CancelFunc()
		grpcClient.Connection.Close()

		grpcClient.Connection = nil
		grpcClient.CallContext = nil
		grpcClient.CancelFunc = nil
		grpcClient.Client = nil
	}
}
