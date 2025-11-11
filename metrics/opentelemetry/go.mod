module github.com/mushtruk/floodgate/metrics/opentelemetry

go 1.24.0

require (
	github.com/mushtruk/floodgate v0.0.0
	go.opentelemetry.io/otel v1.33.0
	go.opentelemetry.io/otel/metric v1.33.0
)

replace github.com/mushtruk/floodgate => ../..
