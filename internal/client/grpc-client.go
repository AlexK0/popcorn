package client

import (
	"context"
	"time"

	pb "github.com/AlexK0/popcorn/internal/api/proto/v1"
	"github.com/AlexK0/popcorn/internal/common"
	"google.golang.org/grpc"
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
	connectionContext, connectionCancel := context.WithTimeout(context.Background(), time.Second*3)
	defer connectionCancel()
	connection, err := grpc.DialContext(
		connectionContext,
		serverHostPort,
		grpc.WithInsecure(),
		grpc.WithBlock(),
		grpc.WithDefaultCallOptions(grpc.UseCompressor(common.ZstdCompressorName)))
	if err != nil {
		return nil, err
	}

	ctx, cancelFunc := context.WithTimeout(context.Background(), time.Minute*5)
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
