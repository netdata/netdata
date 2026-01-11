# 11.4 Network and Connectivity Alerts

Network alerts focus on endpoints and services rather than interface statistics. These alerts ensure that network-dependent services remain reachable.

## Ping and Latency Monitoring

### ping_latency

Tracks round-trip time with thresholds calibrated for typical operational requirements.

**Context:** `ping.latency`
**Thresholds:** WARN > 100ms, CRIT > 500ms

### ping_packet_loss

Measures percentage of packets that do not receive responses. Network problems often manifest as partial packet loss before complete failure.

**Context:** `ping.packets`
**Thresholds:** WARN > 1%, CRIT > 5%

## Port and Service Monitoring

### port_check_failed

Attempts to connect to a specified port and fires when the connection fails. Can monitor any TCP service.

**Context:** `net.port`
**Thresholds:** CRIT not responding

### port_response_time

Tracks how long the connection takes to establish, catching slow services before they fail.

**Context:** `net.port`
**Thresholds:** WARN > 1s

### ssl_certificate_expiry

Monitors certificate validity period with sufficient lead time for renewal.

**Context:** `ssl.cert`
**Thresholds:** WARN < 30 days, CRIT < 7 days

### ssl_handshake_failure

Tracks SSL/TLS handshake failures which may indicate certificate or protocol problems.

**Context:** `ssl.handshake`
**Thresholds:** WARN > 0

## DNS Monitoring

### dns_query_time

Tracks resolution latency for DNS-dependent applications.

**Context:** `dns.query`
**Thresholds:** WARN > 50ms, CRIT > 200ms

### dns_query_failures

Fires when DNS resolution fails entirely, which causes cascading failures in dependent applications.

**Context:** `dns.query`
**Thresholds:** WARN > 0

### dns_no_response

Monitors for complete DNS non-responses.

**Context:** `dns.response`
**Thresholds:** CRIT > 0

## HTTP Endpoint Monitoring

### http_response_code_not_2xx

Tracks non-2xx responses indicating client or server errors.

**Context:** `httpcheck.response`
**Thresholds:** WARN > 0, CRIT > 10%

### http_response_time_percentile

Monitors 95th percentile latency for SLA compliance.

**Context:** `httpcheck.response`
**Thresholds:** WARN > 2s