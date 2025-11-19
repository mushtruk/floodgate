module github.com/mushtruk/floodgate/examples/codel-http

go 1.24.0

require (
	github.com/mushtruk/floodgate v0.0.0
	github.com/mushtruk/floodgate/algorithms/codel v0.0.0
)

replace (
	github.com/mushtruk/floodgate => ../..
	github.com/mushtruk/floodgate/algorithms/codel => ../../algorithms/codel
)
