# WebSphere Metrics Outputs

This directory contains metrics outputs from all WebSphere instances in the test environment.

## File Naming Convention

### Liberty MicroProfile Metrics (Prometheus format)
- `liberty-{version}-mpmetrics-{mpversion}-port-{port}.txt`
- `open-liberty-{version}-mpmetrics-{mpversion}-port-{port}.txt`

### Traditional WebSphere PMI (XML format)
- `traditional-{version}-pmi-full-port-{port}.xml`

## Versions Collected

### Liberty Servers (MicroProfile Metrics - Prometheus format)
| Server | Version | mpMetrics | Port | URL |
|--------|---------|-----------|------|-----|
| Liberty 20 | 20.0.0.12 | 3.0 | 9182 | http://10.20.4.56:9182/metrics |
| Liberty 22 | 22.0.0.13 | 4.0 | 9181 | http://10.20.4.56:9181/metrics |
| Liberty 23 | 23.0.0.12 | 5.1 | 9180 | http://10.20.4.56:9180/metrics |
| Liberty 24 | 24.0.0.12 | 5.1 | 9184 | http://10.20.4.56:9184/metrics |
| Liberty Latest | Latest | 5.1 | 9080 | http://10.20.4.56:9080/metrics |
| Liberty MP | MP | 5.1 | 9081 | http://10.20.4.56:9081/metrics |
| Open Liberty | Latest | 5.1 | 9082 | http://10.20.4.56:9082/metrics |
| Liberty Collective | Collective | 5.0 | 9187 | http://10.20.4.56:9187/metrics |

### Traditional WebSphere (PMI - XML format)
| Server | Version | Port | URL |
|--------|---------|------|-----|
| Traditional 8.5.5 | 8.5.5.24 | 9284 | http://10.20.4.56:9284/wasPerfTool/servlet/perfservlet |
| Traditional 9.0.5 | 9.0.5.x | 9083 | http://10.20.4.56:9083/wasPerfTool/servlet/perfservlet |

## Configuration Notes
- All Liberty servers have `monitor filter="*"` configured for maximum metrics exposure
- All mpMetrics endpoints have authentication disabled for testing purposes
- PMI outputs are in XML format and contain comprehensive performance data
- MicroProfile Metrics outputs are in Prometheus exposition format

## Direct Access URLs
You can access the metrics directly in your browser using the URLs listed above.
