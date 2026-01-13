# 11.4 Network and Connectivity Alerts

Network alerts focus on endpoints and services rather than interface statistics. These alerts ensure that network-dependent services remain reachable.

:::note

Network connectivity alerts require specific endpoints to be configured. Add the hosts, ports, or URLs you want to monitor using the appropriate collector configuration.

:::

## 11.4.1 Ping and Latency Monitoring

### ping_host_latency

Tracks round-trip time with thresholds calibrated for typical operational requirements.

**Context:** `ping.host_rtt`
**Thresholds:** WARN > 500ms, CRIT > 1000ms

### ping_packet_loss

Measures percentage of packets that do not receive responses. Network problems often manifest as partial packet loss before complete failure.

**Context:** `ping.host_packet_loss`
**Thresholds:** WARN > 5%, CRIT > 10%

### ping_host_reachable

Tracks host reachability status.

**Context:** `ping.host_packet_loss`
**Thresholds:** CRIT == 0 (not reachable)

## 11.4.2 Port and Service Monitoring

### portcheck_connection_fails

Attempts to connect to a specified port and fires when the connection fails. Can monitor any TCP service.

**Context:** `portcheck.status`
**Thresholds:** CRIT > 40% failed

### portcheck_connection_timeouts

Tracks how long connections take to establish, catching slow services before they fail.

**Context:** `portcheck.status`
**Thresholds:** WARN > 10% timeout, CRIT > 40% timeout

### portcheck_service_reachable

Tracks port/service reachability status.

**Context:** `portcheck.status`
**Thresholds:** CRIT < 75% success

## 11.4.3 SSL Certificate Monitoring

### x509check_days_until_expiration

Monitors certificate validity period with sufficient lead time for renewal.

**Context:** `x509check.time_until_expiration`
**Thresholds:** WARN < 14 days, CRIT < 7 days

### x509check_revocation_status

Tracks SSL/TLS certificate revocation status.

**Context:** `x509check.revocation_status`
**Thresholds:** CRIT revoked

## 11.4.4 DNS Monitoring

### dns_query_query_status

Fires when DNS resolution fails entirely, which causes cascading failures in dependent applications.

**Context:** `dns_query.query_status`
**Thresholds:** WARN != 1 (failed)

## 11.4.5 HTTP Endpoint Monitoring

### httpcheck_web_service_bad_status

Tracks non-2xx responses indicating client or server errors.

**Context:** `httpcheck.status`
**Thresholds:** WARN > 0 bad status, CRIT > 10% bad status

### httpcheck_web_service_timeouts

Monitors HTTP request timeouts.

**Context:** `httpcheck.status`
**Thresholds:** WARN > 0 timeouts

### httpcheck_web_service_up

Tracks overall HTTP service availability.

**Context:** `httpcheck.status`
**Thresholds:** CRIT not responding

### httpcheck_web_service_bad_content

Tracks when HTTP responses don't match expected content patterns.

**Context:** `httpcheck.status`
**Thresholds:** CRIT bad content

## Related Sections

- [11.1 Application Alerts](./3-application-alerts.md) - Database, web server, cache, and message queue alerts
- [11.2 Container Alerts](./2-container-alerts.md) - Docker and Kubernetes monitoring
- [11.3 Hardware Alerts](./5-hardware-alerts.md) - Physical server and storage device alerts
- [11.5 System Resource Alerts](./1-system-resource-alerts.md) - CPU, memory, disk, and load alerts