#!/bin/sh

sudo apt install protobuf-compiler

export GO111MODULE=on  # Enable module mode
go get -u google.golang.org/grpc
go get -u google.golang.org/protobuf/cmd/protoc-gen-go 
go get -u google.golang.org/grpc/cmd/protoc-gen-go-grpc
