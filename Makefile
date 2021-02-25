RELEASE = v0.0.1
BUILD_COMMIT := $(shell git rev-parse --short HEAD)
DATE := $(shell date --utc '+%F %X UTC')
VERSION := ${RELEASE}, rev ${BUILD_COMMIT}, compiled at ${DATE}
GOPATH := $(shell go env GOPATH)

.EXPORT_ALL_VARIABLES:
PATH := ${PATH}:${GOPATH}/bin

protogen:
	protoc --go_out=internal/ --go_opt=paths=source_relative --go-grpc_out=internal/ --go-grpc_opt=paths=source_relative  api/proto/v1/compilation-server.proto

check: protogen
	golangci-lint run

client: protogen
	go build -o bin/popcorn-client -ldflags '-X "github.com/AlexK0/popcorn/internal/common.version=${VERSION}"' cmd/popcorn-client/main.go

server: protogen
	go build -o bin/popcorn-server -ldflags '-X "github.com/AlexK0/popcorn/internal/common.version=${VERSION}"' cmd/popcorn-server/main.go

.DEFAULT_GOAL := all
all: check client server
.PHONY : all

clean:
	rm bin/popcorn-client bin/popcorn-server
