module github.com/rootkernel/gul/contract

go 1.27.0

toolchain go1.27.1

require (
	connectrpc.com/connect v1.20.0
	google.golang.org/protobuf v1.36.12
)

tool (
	connectrpc.com/connect/cmd/protoc-gen-connect-go
	google.golang.org/protobuf/cmd/protoc-gen-go
)
