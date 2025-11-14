module github.com/mushtruk/floodgate/tracing

go 1.24.0

require (
	github.com/mushtruk/floodgate v0.0.0
	go.opentelemetry.io/otel v1.37.0
	go.opentelemetry.io/otel/trace v1.37.0
)

replace github.com/mushtruk/floodgate => ..
