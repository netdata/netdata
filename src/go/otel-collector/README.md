# Netdata OpenTelemetry Collector

A custom distribution of the OpenTelemetry Collector maintained by Netdata.

Based on the official OpenTelemetry Collector, this distribution maintains upstream compatibility while providing specialized components designed for Netdata environments. All custom components are maintained and distributed within this repository.

## Build

```bash
go install go.opentelemetry.io/collector/cmd/builder@latest
cd src/go/otel-collector
builder --config=builder-config.yaml
build/otelcol.plugin
```
