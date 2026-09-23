module github.com/rootkernel/gul

go 1.26.0

toolchain go1.26.6

require (
	connectrpc.com/connect v1.20.0
	github.com/rootkernel/gul/contract v0.0.0
	google.golang.org/protobuf v1.36.12
)

replace github.com/rootkernel/gul/contract => ./contract
