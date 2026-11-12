module github.com/mushtruk/floodgate/examples/middleware-chain

go 1.24.0

toolchain go1.24.2

require github.com/mushtruk/floodgate v1.6.1 // x-release-please-version

require github.com/hashicorp/golang-lru/v2 v2.0.7 // indirect

replace github.com/mushtruk/floodgate => ../..
