# Changelog

## [1.6.1](https://github.com/mushtruk/floodgate/compare/v1.6.0...v1.6.1) (2026-11-12)


### Bug Fixes

* **build:** require the released root version in every module ([f3b6c17](https://github.com/mushtruk/floodgate/commit/f3b6c175a6e705a5826cfcc9a767526b6c7e686d))
* **metrics:** never report a wrapped counter delta after a reset ([b4b1c61](https://github.com/mushtruk/floodgate/commit/b4b1c61c1230edce1439586b4618e39e38f37a93))

## [1.6.0](https://github.com/mushtruk/floodgate/compare/v1.5.0...v1.6.0) (2025-12-01)


### BREAKING CHANGES

* **codel:** `codel.NewAlgorithm` and `codel.NewQueueAlgorithm` return an error for an invalid configuration instead of panicking.
* **http,grpc:** `SkipPaths` and `SkipMethods` entries match exactly; only an entry ending in `*` matches as a prefix. Configurations that relied on implicit prefix matching need a trailing `*`.

### Bug Fixes

* **core:** populate the method label in latency metrics.
* **http:** capture the response status code for metrics.
* **metrics:** make `CompositeMetrics` safe for concurrent use.
* **tracker:** make the percentile cache validity flag safe for concurrent use.

### Performance Improvements

* **codel:** remove lock contention from the fast path.

### Notes

* The submodules `metrics/prometheus`, `metrics/datadog`, `metrics/opentelemetry`, `tracing` and `algorithms/codel` required `github.com/mushtruk/floodgate v0.0.0` in every release up to and including 1.6.0, so none of them could be installed outside this repository before 1.6.1. The codel change above therefore reached no external user. `algorithms/codel` was removed without ever having been installable.
* 1.6.0 shipped as a minor release although the `SkipPaths` and `SkipMethods` change above breaks existing configurations of the root module, which violates SemVer: a breaking change requires a major version.

## [1.5.0](https://github.com/mushtruk/floodgate/compare/v1.4.1...v1.5.0) (2025-11-23)


### Features

* pluggable `Algorithm` interface with `ThresholdAlgorithm` (the previous threshold logic) and `NoOpAlgorithm`.
* **codel:** CoDel (Controlled Delay) algorithm in `algorithms/codel`.
* algorithm wrappers: `WithTracing`, `WithFallback`, `NewCachedAlgorithm` and `NewInstrumentedAlgorithm`.
* circuit breaker wrappers: `WithLogging`, `WithMetrics`, `WithAlerting` and `NewInstrumentedCircuitBreaker`.
* HTTP middleware and gRPC interceptor share one protocol-agnostic core.
* examples for algorithm selection and middleware chaining.

### Bug Fixes

* **codel:** reset the above-target timestamp whenever the delay drops below target, so a transient spike no longer triggers dropping later.

### Notes

* The v1.5.0 tag points to ae7604a on the feature branch, not to the merge commit 644189b on master. Both have the same tree.

## [1.4.1](https://github.com/mushtruk/floodgate/compare/v1.4.0...v1.4.1) (2025-11-16)


### Notes

* This release contains lint fixes, CI workflows and documentation changes only. The API did not change.

## [1.4.0](https://github.com/mushtruk/floodgate/compare/v1.3.0...v1.4.0) (2025-11-15)


### Features

* **metrics/datadog:** Datadog metrics through DogStatsD.
* **tracing:** OpenTelemetry tracing of backpressure decisions, rejections and circuit breaker state.

## [1.3.0](https://github.com/mushtruk/floodgate/compare/v1.2.0...v1.3.0) (2025-11-11)


### Features

* pluggable `MetricsCollector` interface with a no-op default, collected automatically by the HTTP middleware and gRPC interceptor.
* **metrics/prometheus:** Prometheus metrics.
* **metrics/opentelemetry:** OpenTelemetry metrics.

## [1.2.0](https://github.com/mushtruk/floodgate/compare/v1.1.0...v1.2.0) (2025-11-09)


### Features

* configurable context-aware `Logger` interface with standard library, `slog` and no-op implementations.

## [1.1.0](https://github.com/mushtruk/floodgate/compare/v1.0.0...v1.1.0) (2025-11-08)


### Features

* **http:** HTTP middleware for adaptive backpressure.

## [1.0.0](https://github.com/mushtruk/floodgate/tree/v1.0.0) (2025-10-22)


### Features

* adaptive backpressure from EMA and percentile (P50, P95, P99) latency tracking.
* circuit breaker against cascading failures.
* **grpc:** gRPC server interceptor with load shedding.

### Notes

* The v1.0.0 tag points to c29ad4a, which is not on master. Master starts from 4946e88, which has the same code and differs only in documentation (BENCHMARKS.md and README.md).
