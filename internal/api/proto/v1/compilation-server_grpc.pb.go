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
	CopyHeadersFromClientCache(ctx context.Context, in *CopyHeadersFromClientCacheRequest, opts ...grpc.CallOption) (CompilationService_CopyHeadersFromClientCacheClient, error)
	CopyHeadersFromGlobalCache(ctx context.Context, in *CopyHeadersFromGlobalCacheRequest, opts ...grpc.CallOption) (CompilationService_CopyHeadersFromGlobalCacheClient, error)
	CopyHeader(ctx context.Context, in *CopyHeaderRequest, opts ...grpc.CallOption) (*CopyHeaderReply, error)
	CompileSource(ctx context.Context, in *CompileSourceRequest, opts ...grpc.CallOption) (*CompileSourceReply, error)
	Status(ctx context.Context, in *StatusRequest, opts ...grpc.CallOption) (*StatusReply, error)
	UpdateServer(ctx context.Context, in *UpdateServerRequest, opts ...grpc.CallOption) (*UpdateServerReply, error)
}

type compilationServiceClient struct {
	cc grpc.ClientConnInterface
}

func NewCompilationServiceClient(cc grpc.ClientConnInterface) CompilationServiceClient {
	return &compilationServiceClient{cc}
}

func (c *compilationServiceClient) CopyHeadersFromClientCache(ctx context.Context, in *CopyHeadersFromClientCacheRequest, opts ...grpc.CallOption) (CompilationService_CopyHeadersFromClientCacheClient, error) {
	stream, err := c.cc.NewStream(ctx, &CompilationService_ServiceDesc.Streams[0], "/popcorn.CompilationService/CopyHeadersFromClientCache", opts...)
	if err != nil {
		return nil, err
	}
	x := &compilationServiceCopyHeadersFromClientCacheClient{stream}
	if err := x.ClientStream.SendMsg(in); err != nil {
		return nil, err
	}
	if err := x.ClientStream.CloseSend(); err != nil {
		return nil, err
	}
	return x, nil
}

type CompilationService_CopyHeadersFromClientCacheClient interface {
	Recv() (*CopyHeadersFromClientCacheReply, error)
	grpc.ClientStream
}

type compilationServiceCopyHeadersFromClientCacheClient struct {
	grpc.ClientStream
}

func (x *compilationServiceCopyHeadersFromClientCacheClient) Recv() (*CopyHeadersFromClientCacheReply, error) {
	m := new(CopyHeadersFromClientCacheReply)
	if err := x.ClientStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

func (c *compilationServiceClient) CopyHeadersFromGlobalCache(ctx context.Context, in *CopyHeadersFromGlobalCacheRequest, opts ...grpc.CallOption) (CompilationService_CopyHeadersFromGlobalCacheClient, error) {
	stream, err := c.cc.NewStream(ctx, &CompilationService_ServiceDesc.Streams[1], "/popcorn.CompilationService/CopyHeadersFromGlobalCache", opts...)
	if err != nil {
		return nil, err
	}
	x := &compilationServiceCopyHeadersFromGlobalCacheClient{stream}
	if err := x.ClientStream.SendMsg(in); err != nil {
		return nil, err
	}
	if err := x.ClientStream.CloseSend(); err != nil {
		return nil, err
	}
	return x, nil
}

type CompilationService_CopyHeadersFromGlobalCacheClient interface {
	Recv() (*CopyHeadersFromGlobalCacheReply, error)
	grpc.ClientStream
}

type compilationServiceCopyHeadersFromGlobalCacheClient struct {
	grpc.ClientStream
}

func (x *compilationServiceCopyHeadersFromGlobalCacheClient) Recv() (*CopyHeadersFromGlobalCacheReply, error) {
	m := new(CopyHeadersFromGlobalCacheReply)
	if err := x.ClientStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

func (c *compilationServiceClient) CopyHeader(ctx context.Context, in *CopyHeaderRequest, opts ...grpc.CallOption) (*CopyHeaderReply, error) {
	out := new(CopyHeaderReply)
	err := c.cc.Invoke(ctx, "/popcorn.CompilationService/CopyHeader", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *compilationServiceClient) CompileSource(ctx context.Context, in *CompileSourceRequest, opts ...grpc.CallOption) (*CompileSourceReply, error) {
	out := new(CompileSourceReply)
	err := c.cc.Invoke(ctx, "/popcorn.CompilationService/CompileSource", in, out, opts...)
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

func (c *compilationServiceClient) UpdateServer(ctx context.Context, in *UpdateServerRequest, opts ...grpc.CallOption) (*UpdateServerReply, error) {
	out := new(UpdateServerReply)
	err := c.cc.Invoke(ctx, "/popcorn.CompilationService/UpdateServer", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// CompilationServiceServer is the server API for CompilationService service.
// All implementations must embed UnimplementedCompilationServiceServer
// for forward compatibility
type CompilationServiceServer interface {
	CopyHeadersFromClientCache(*CopyHeadersFromClientCacheRequest, CompilationService_CopyHeadersFromClientCacheServer) error
	CopyHeadersFromGlobalCache(*CopyHeadersFromGlobalCacheRequest, CompilationService_CopyHeadersFromGlobalCacheServer) error
	CopyHeader(context.Context, *CopyHeaderRequest) (*CopyHeaderReply, error)
	CompileSource(context.Context, *CompileSourceRequest) (*CompileSourceReply, error)
	Status(context.Context, *StatusRequest) (*StatusReply, error)
	UpdateServer(context.Context, *UpdateServerRequest) (*UpdateServerReply, error)
	mustEmbedUnimplementedCompilationServiceServer()
}

// UnimplementedCompilationServiceServer must be embedded to have forward compatible implementations.
type UnimplementedCompilationServiceServer struct {
}

func (UnimplementedCompilationServiceServer) CopyHeadersFromClientCache(*CopyHeadersFromClientCacheRequest, CompilationService_CopyHeadersFromClientCacheServer) error {
	return status.Errorf(codes.Unimplemented, "method CopyHeadersFromClientCache not implemented")
}
func (UnimplementedCompilationServiceServer) CopyHeadersFromGlobalCache(*CopyHeadersFromGlobalCacheRequest, CompilationService_CopyHeadersFromGlobalCacheServer) error {
	return status.Errorf(codes.Unimplemented, "method CopyHeadersFromGlobalCache not implemented")
}
func (UnimplementedCompilationServiceServer) CopyHeader(context.Context, *CopyHeaderRequest) (*CopyHeaderReply, error) {
	return nil, status.Errorf(codes.Unimplemented, "method CopyHeader not implemented")
}
func (UnimplementedCompilationServiceServer) CompileSource(context.Context, *CompileSourceRequest) (*CompileSourceReply, error) {
	return nil, status.Errorf(codes.Unimplemented, "method CompileSource not implemented")
}
func (UnimplementedCompilationServiceServer) Status(context.Context, *StatusRequest) (*StatusReply, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Status not implemented")
}
func (UnimplementedCompilationServiceServer) UpdateServer(context.Context, *UpdateServerRequest) (*UpdateServerReply, error) {
	return nil, status.Errorf(codes.Unimplemented, "method UpdateServer not implemented")
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

func _CompilationService_CopyHeadersFromClientCache_Handler(srv interface{}, stream grpc.ServerStream) error {
	m := new(CopyHeadersFromClientCacheRequest)
	if err := stream.RecvMsg(m); err != nil {
		return err
	}
	return srv.(CompilationServiceServer).CopyHeadersFromClientCache(m, &compilationServiceCopyHeadersFromClientCacheServer{stream})
}

type CompilationService_CopyHeadersFromClientCacheServer interface {
	Send(*CopyHeadersFromClientCacheReply) error
	grpc.ServerStream
}

type compilationServiceCopyHeadersFromClientCacheServer struct {
	grpc.ServerStream
}

func (x *compilationServiceCopyHeadersFromClientCacheServer) Send(m *CopyHeadersFromClientCacheReply) error {
	return x.ServerStream.SendMsg(m)
}

func _CompilationService_CopyHeadersFromGlobalCache_Handler(srv interface{}, stream grpc.ServerStream) error {
	m := new(CopyHeadersFromGlobalCacheRequest)
	if err := stream.RecvMsg(m); err != nil {
		return err
	}
	return srv.(CompilationServiceServer).CopyHeadersFromGlobalCache(m, &compilationServiceCopyHeadersFromGlobalCacheServer{stream})
}

type CompilationService_CopyHeadersFromGlobalCacheServer interface {
	Send(*CopyHeadersFromGlobalCacheReply) error
	grpc.ServerStream
}

type compilationServiceCopyHeadersFromGlobalCacheServer struct {
	grpc.ServerStream
}

func (x *compilationServiceCopyHeadersFromGlobalCacheServer) Send(m *CopyHeadersFromGlobalCacheReply) error {
	return x.ServerStream.SendMsg(m)
}

func _CompilationService_CopyHeader_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(CopyHeaderRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(CompilationServiceServer).CopyHeader(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/popcorn.CompilationService/CopyHeader",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(CompilationServiceServer).CopyHeader(ctx, req.(*CopyHeaderRequest))
	}
	return interceptor(ctx, in, info, handler)
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

func _CompilationService_UpdateServer_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(UpdateServerRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(CompilationServiceServer).UpdateServer(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/popcorn.CompilationService/UpdateServer",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(CompilationServiceServer).UpdateServer(ctx, req.(*UpdateServerRequest))
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
			MethodName: "CopyHeader",
			Handler:    _CompilationService_CopyHeader_Handler,
		},
		{
			MethodName: "CompileSource",
			Handler:    _CompilationService_CompileSource_Handler,
		},
		{
			MethodName: "Status",
			Handler:    _CompilationService_Status_Handler,
		},
		{
			MethodName: "UpdateServer",
			Handler:    _CompilationService_UpdateServer_Handler,
		},
	},
	Streams: []grpc.StreamDesc{
		{
			StreamName:    "CopyHeadersFromClientCache",
			Handler:       _CompilationService_CopyHeadersFromClientCache_Handler,
			ServerStreams: true,
		},
		{
			StreamName:    "CopyHeadersFromGlobalCache",
			Handler:       _CompilationService_CopyHeadersFromGlobalCache_Handler,
			ServerStreams: true,
		},
	},
	Metadata: "api/proto/v1/compilation-server.proto",
}
