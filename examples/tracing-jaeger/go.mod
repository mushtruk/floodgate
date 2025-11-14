module github.com/mushtruk/floodgate/examples/tracing-jaeger

go 1.24.0

require (
	github.com/mushtruk/floodgate v0.0.0
	github.com/mushtruk/floodgate/tracing v0.0.0
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.58.0
	go.opentelemetry.io/otel v1.37.0
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp v1.37.0
	go.opentelemetry.io/otel/sdk v1.37.0
	go.opentelemetry.io/otel/trace v1.37.0
)

require (
	github.com/cenkalti/backoff/v5 v5.0.2 // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.27.1 // indirect
	github.com/hashicorp/golang-lru/v2 v2.0.7 // indirect
	go.opentelemetry.io/auto/sdk v1.1.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.37.0 // indirect
	go.opentelemetry.io/otel/metric v1.37.0 // indirect
	go.opentelemetry.io/proto/otlp v1.7.0 // indirect
	golang.org/x/net v0.46.0 // indirect
	golang.org/x/sys v0.38.0 // indirect
	golang.org/x/text v0.30.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20250804133106-a7a43d27e69b // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20251110190251-83f479183930 // indirect
	google.golang.org/grpc v1.76.0 // indirect
	google.golang.org/protobuf v1.36.10 // indirect
)

replace github.com/mushtruk/floodgate => ../..

replace github.com/mushtruk/floodgate/tracing => ../../tracing
