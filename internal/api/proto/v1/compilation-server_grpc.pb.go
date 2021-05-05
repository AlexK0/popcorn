// Code generated by protoc-gen-go-grpc. DO NOT EDIT.

package popcorn

import (
	context "context"
	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
)

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
// Requires gRPC-Go v1.32.0 or later.
const _ = grpc.SupportPackageIsVersion7

// CompilationServiceClient is the client API for CompilationService service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
type CompilationServiceClient interface {
	// Compilation api
	StartCompilationSession(ctx context.Context, in *StartCompilationSessionRequest, opts ...grpc.CallOption) (*StartCompilationSessionReply, error)
	SendHeaderSHA256(ctx context.Context, in *SendHeaderSHA256Request, opts ...grpc.CallOption) (*SendHeaderSHA256Reply, error)
	SendHeader(ctx context.Context, opts ...grpc.CallOption) (CompilationService_SendHeaderClient, error)
	CompileSource(ctx context.Context, in *CompileSourceRequest, opts ...grpc.CallOption) (*CompileSourceReply, error)
	CloseSession(ctx context.Context, in *CloseSessionRequest, opts ...grpc.CallOption) (*CloseSessionReply, error)
	// Service api
	Status(ctx context.Context, in *StatusRequest, opts ...grpc.CallOption) (*StatusReply, error)
}

type compilationServiceClient struct {
	cc grpc.ClientConnInterface
}

func NewCompilationServiceClient(cc grpc.ClientConnInterface) CompilationServiceClient {
	return &compilationServiceClient{cc}
}

func (c *compilationServiceClient) StartCompilationSession(ctx context.Context, in *StartCompilationSessionRequest, opts ...grpc.CallOption) (*StartCompilationSessionReply, error) {
	out := new(StartCompilationSessionReply)
	err := c.cc.Invoke(ctx, "/popcorn.CompilationService/StartCompilationSession", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *compilationServiceClient) SendHeaderSHA256(ctx context.Context, in *SendHeaderSHA256Request, opts ...grpc.CallOption) (*SendHeaderSHA256Reply, error) {
	out := new(SendHeaderSHA256Reply)
	err := c.cc.Invoke(ctx, "/popcorn.CompilationService/SendHeaderSHA256", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *compilationServiceClient) SendHeader(ctx context.Context, opts ...grpc.CallOption) (CompilationService_SendHeaderClient, error) {
	stream, err := c.cc.NewStream(ctx, &CompilationService_ServiceDesc.Streams[0], "/popcorn.CompilationService/SendHeader", opts...)
	if err != nil {
		return nil, err
	}
	x := &compilationServiceSendHeaderClient{stream}
	return x, nil
}

type CompilationService_SendHeaderClient interface {
	Send(*SendHeaderRequest) error
	CloseAndRecv() (*SendHeaderReply, error)
	grpc.ClientStream
}

type compilationServiceSendHeaderClient struct {
	grpc.ClientStream
}

func (x *compilationServiceSendHeaderClient) Send(m *SendHeaderRequest) error {
	return x.ClientStream.SendMsg(m)
}

func (x *compilationServiceSendHeaderClient) CloseAndRecv() (*SendHeaderReply, error) {
	if err := x.ClientStream.CloseSend(); err != nil {
		return nil, err
	}
	m := new(SendHeaderReply)
	if err := x.ClientStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

func (c *compilationServiceClient) CompileSource(ctx context.Context, in *CompileSourceRequest, opts ...grpc.CallOption) (*CompileSourceReply, error) {
	out := new(CompileSourceReply)
	err := c.cc.Invoke(ctx, "/popcorn.CompilationService/CompileSource", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *compilationServiceClient) CloseSession(ctx context.Context, in *CloseSessionRequest, opts ...grpc.CallOption) (*CloseSessionReply, error) {
	out := new(CloseSessionReply)
	err := c.cc.Invoke(ctx, "/popcorn.CompilationService/CloseSession", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *compilationServiceClient) Status(ctx context.Context, in *StatusRequest, opts ...grpc.CallOption) (*StatusReply, error) {
	out := new(StatusReply)
	err := c.cc.Invoke(ctx, "/popcorn.CompilationService/Status", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// CompilationServiceServer is the server API for CompilationService service.
// All implementations must embed UnimplementedCompilationServiceServer
// for forward compatibility
type CompilationServiceServer interface {
	// Compilation api
	StartCompilationSession(context.Context, *StartCompilationSessionRequest) (*StartCompilationSessionReply, error)
	SendHeaderSHA256(context.Context, *SendHeaderSHA256Request) (*SendHeaderSHA256Reply, error)
	SendHeader(CompilationService_SendHeaderServer) error
	CompileSource(context.Context, *CompileSourceRequest) (*CompileSourceReply, error)
	CloseSession(context.Context, *CloseSessionRequest) (*CloseSessionReply, error)
	// Service api
	Status(context.Context, *StatusRequest) (*StatusReply, error)
	mustEmbedUnimplementedCompilationServiceServer()
}

// UnimplementedCompilationServiceServer must be embedded to have forward compatible implementations.
type UnimplementedCompilationServiceServer struct {
}

func (UnimplementedCompilationServiceServer) StartCompilationSession(context.Context, *StartCompilationSessionRequest) (*StartCompilationSessionReply, error) {
	return nil, status.Errorf(codes.Unimplemented, "method StartCompilationSession not implemented")
}
func (UnimplementedCompilationServiceServer) SendHeaderSHA256(context.Context, *SendHeaderSHA256Request) (*SendHeaderSHA256Reply, error) {
	return nil, status.Errorf(codes.Unimplemented, "method SendHeaderSHA256 not implemented")
}
func (UnimplementedCompilationServiceServer) SendHeader(CompilationService_SendHeaderServer) error {
	return status.Errorf(codes.Unimplemented, "method SendHeader not implemented")
}
func (UnimplementedCompilationServiceServer) CompileSource(context.Context, *CompileSourceRequest) (*CompileSourceReply, error) {
	return nil, status.Errorf(codes.Unimplemented, "method CompileSource not implemented")
}
func (UnimplementedCompilationServiceServer) CloseSession(context.Context, *CloseSessionRequest) (*CloseSessionReply, error) {
	return nil, status.Errorf(codes.Unimplemented, "method CloseSession not implemented")
}
func (UnimplementedCompilationServiceServer) Status(context.Context, *StatusRequest) (*StatusReply, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Status not implemented")
}
func (UnimplementedCompilationServiceServer) mustEmbedUnimplementedCompilationServiceServer() {}

// UnsafeCompilationServiceServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to CompilationServiceServer will
// result in compilation errors.
type UnsafeCompilationServiceServer interface {
	mustEmbedUnimplementedCompilationServiceServer()
}

func RegisterCompilationServiceServer(s grpc.ServiceRegistrar, srv CompilationServiceServer) {
	s.RegisterService(&CompilationService_ServiceDesc, srv)
}

func _CompilationService_StartCompilationSession_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(StartCompilationSessionRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(CompilationServiceServer).StartCompilationSession(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/popcorn.CompilationService/StartCompilationSession",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(CompilationServiceServer).StartCompilationSession(ctx, req.(*StartCompilationSessionRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _CompilationService_SendHeaderSHA256_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(SendHeaderSHA256Request)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(CompilationServiceServer).SendHeaderSHA256(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/popcorn.CompilationService/SendHeaderSHA256",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(CompilationServiceServer).SendHeaderSHA256(ctx, req.(*SendHeaderSHA256Request))
	}
	return interceptor(ctx, in, info, handler)
}

func _CompilationService_SendHeader_Handler(srv interface{}, stream grpc.ServerStream) error {
	return srv.(CompilationServiceServer).SendHeader(&compilationServiceSendHeaderServer{stream})
}

type CompilationService_SendHeaderServer interface {
	SendAndClose(*SendHeaderReply) error
	Recv() (*SendHeaderRequest, error)
	grpc.ServerStream
}

type compilationServiceSendHeaderServer struct {
	grpc.ServerStream
}

func (x *compilationServiceSendHeaderServer) SendAndClose(m *SendHeaderReply) error {
	return x.ServerStream.SendMsg(m)
}

func (x *compilationServiceSendHeaderServer) Recv() (*SendHeaderRequest, error) {
	m := new(SendHeaderRequest)
	if err := x.ServerStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

func _CompilationService_CompileSource_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(CompileSourceRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(CompilationServiceServer).CompileSource(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/popcorn.CompilationService/CompileSource",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(CompilationServiceServer).CompileSource(ctx, req.(*CompileSourceRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _CompilationService_CloseSession_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(CloseSessionRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(CompilationServiceServer).CloseSession(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/popcorn.CompilationService/CloseSession",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(CompilationServiceServer).CloseSession(ctx, req.(*CloseSessionRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _CompilationService_Status_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(StatusRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(CompilationServiceServer).Status(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/popcorn.CompilationService/Status",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(CompilationServiceServer).Status(ctx, req.(*StatusRequest))
	}
	return interceptor(ctx, in, info, handler)
}

// CompilationService_ServiceDesc is the grpc.ServiceDesc for CompilationService service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var CompilationService_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "popcorn.CompilationService",
	HandlerType: (*CompilationServiceServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "StartCompilationSession",
			Handler:    _CompilationService_StartCompilationSession_Handler,
		},
		{
			MethodName: "SendHeaderSHA256",
			Handler:    _CompilationService_SendHeaderSHA256_Handler,
		},
		{
			MethodName: "CompileSource",
			Handler:    _CompilationService_CompileSource_Handler,
		},
		{
			MethodName: "CloseSession",
			Handler:    _CompilationService_CloseSession_Handler,
		},
		{
			MethodName: "Status",
			Handler:    _CompilationService_Status_Handler,
		},
	},
	Streams: []grpc.StreamDesc{
		{
			StreamName:    "SendHeader",
			Handler:       _CompilationService_SendHeader_Handler,
			ClientStreams: true,
		},
	},
	Metadata: "api/proto/v1/compilation-server.proto",
}
