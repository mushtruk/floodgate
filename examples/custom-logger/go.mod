module github.com/mushtruk/floodgate/examples/custom-logger

go 1.24.0

replace github.com/mushtruk/floodgate => ../..

require (
	github.com/mushtruk/floodgate v0.0.0-00010101000000-000000000000
	github.com/rs/zerolog v1.33.0
)

require (
	github.com/hashicorp/golang-lru/v2 v2.0.7 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.19 // indirect
	golang.org/x/sys v0.37.0 // indirect
)
