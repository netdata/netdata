# portcheck

Module monitors a remote TCP service.

Following charts are drawn per host:

1. **Latency** ms
 * Time required to connect to a TCP port.
   Displays latency in 0.1 ms resolution. If the connection failed, the value is missing.

2. **Status** boolean
 * Connection successful
 * Could not create socket: possible DNS problems
 * Connection refused: port not listening or blocked
 * Connection timed out: host or port unreachable


### configuration

```yaml
server:
  host: 'dns or ip'     # required
  port: 22              # required
  timeout: 1            # optional
  update_every: 1       # optional
```

### notes

 * The error chart is intended for alarms, badges or for access via API.
 * A system/service/firewall might block netdata's access if a portscan or
   similar is detected.
 * Currently, the accuracy of the latency is low and should be used as reference only.

---
