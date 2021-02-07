protogen:
	protoc --go_out=internal/ --go_opt=paths=source_relative --go-grpc_out=internal/ --go-grpc_opt=paths=source_relative  api/proto/v1/compilation-server.proto

check: protogen
	golangci-lint run

client: protogen
	go build -o popcorn-client cmd/popcorn-client/main.go

server: protogen
	go build -o popcorn-server cmd/popcorn-server/main.go

.DEFAULT_GOAL := all
all: check client server
.PHONY : all

clean:
	rm internal/api/proto/v1/* popcorn-client popcorn-server
