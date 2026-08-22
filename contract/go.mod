module github.com/rootkernel/gul/contract

go 1.26.0

require (
	connectrpc.com/connect v1.20.0 // indirect
	google.golang.org/protobuf v1.36.12 // indirect
)

tool (
	connectrpc.com/connect/cmd/protoc-gen-connect-go
	google.golang.org/protobuf/cmd/protoc-gen-go
)
