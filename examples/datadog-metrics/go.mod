module github.com/mushtruk/floodgate/examples/datadog-metrics

go 1.24.0

require (
	github.com/DataDog/datadog-go/v5 v5.5.0
	github.com/mushtruk/floodgate v0.0.0
	github.com/mushtruk/floodgate/metrics/datadog v0.0.0
)

require (
	github.com/Microsoft/go-winio v0.5.0 // indirect
	github.com/hashicorp/golang-lru/v2 v2.0.7 // indirect
	golang.org/x/sys v0.38.0 // indirect
)

replace github.com/mushtruk/floodgate => ../..

replace github.com/mushtruk/floodgate/metrics/datadog => ../../metrics/datadog
