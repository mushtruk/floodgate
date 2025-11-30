module github.com/mushtruk/floodgate/examples/algorithm-selection

go 1.24.0

toolchain go1.24.2

require (
	github.com/mushtruk/floodgate v0.0.0
	github.com/mushtruk/floodgate/algorithms/codel v0.0.0
)

require github.com/hashicorp/golang-lru/v2 v2.0.7 // indirect

replace (
	github.com/mushtruk/floodgate => ../..
	github.com/mushtruk/floodgate/algorithms/codel => ../../algorithms/codel
)
