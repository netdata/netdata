# 11.4 Network and Connectivity Alerts

Network alerts focus on endpoints and services rather than interface statistics. These alerts ensure that network-dependent services remain reachable.

:::note
Network connectivity alerts require the appropriate collector (ping, httpcheck, port check) to be enabled for the monitored services.
:::

## 11.4.1 Ping and Latency Monitoring

### ping_latency

Tracks round-trip time with thresholds calibrated for typical operational requirements.

| Context | Thresholds |
|---------|------------|
| `ping.latency` | WARN > 100ms, CRIT > 500ms |

### ping_packet_loss

Measures percentage of packets that do not receive responses. Network problems often manifest as partial packet loss before complete failure.

| Context | Thresholds |
|---------|------------|
| `ping.packets` | WARN > 1%, CRIT > 5% |

## 11.4.2 Port and Service Monitoring

### port_check_failed

Attempts to connect to a specified port and fires when the connection fails. Can monitor any TCP service.

| Context | Thresholds |
|---------|------------|
| `net.port` | CRIT not responding |

### port_response_time

Tracks how long the connection takes to establish, catching slow services before they fail.

| Context | Thresholds |
|---------|------------|
| `net.port` | WARN > 1s |

### ssl_certificate_expiry

Monitors certificate validity period with sufficient lead time for renewal.

| Context | Thresholds |
|---------|------------|
| `ssl.cert` | WARN < 30 days, CRIT < 7 days |

### ssl_handshake_failure

Tracks SSL/TLS handshake failures which may indicate certificate or protocol problems.

| Context | Thresholds |
|---------|------------|
| `ssl.handshake` | WARN > 0 |

## 11.4.3 DNS Monitoring

### dns_query_time

Tracks resolution latency for DNS-dependent applications.

| Context | Thresholds |
|---------|------------|
| `dns.query` | WARN > 50ms, CRIT > 200ms |

### dns_query_failures

Fires when DNS resolution fails entirely, which causes cascading failures in dependent applications.

| Context | Thresholds |
|---------|------------|
| `dns.query` | WARN > 0 |

### dns_no_response

Monitors for complete DNS non-responses.

| Context | Thresholds |
|---------|------------|
| `dns.response` | CRIT > 0 |

## 11.4.4 HTTP Endpoint Monitoring

### http_response_code_not_2xx

Tracks non-2xx responses indicating client or server errors.

| Context | Thresholds |
|---------|------------|
| `httpcheck.response` | WARN > 0, CRIT > 10% |

### http_response_time_percentile

Monitors 95th percentile latency for SLA compliance.

| Context | Thresholds |
|---------|------------|
| `httpcheck.response` | WARN > 2s |

## Related Sections

- [11.1 System Resource Alerts](system-resource-alerts.md) - CPU, memory, disk
- [11.3 Application Alerts](application-alerts.md) - Database, web server alerts
- [11.5 Hardware Alerts](hardware-alerts.md) - BMC/IPMI monitoring
- [11.5 Container Alerts](container-alerts.md) - Container network metrics