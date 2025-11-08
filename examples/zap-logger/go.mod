module github.com/mushtruk/floodgate/examples/zap-logger

go 1.24.0

replace github.com/mushtruk/floodgate => ../..

require (
	github.com/mushtruk/floodgate v0.0.0-00010101000000-000000000000
	go.uber.org/zap v1.27.0
)

require (
	github.com/hashicorp/golang-lru/v2 v2.0.7 // indirect
	go.uber.org/multierr v1.11.0 // indirect
)
