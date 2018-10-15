# ntpd

Module monitors the system variables of the local `ntpd` daemon (optional incl. variables of the polled peers) using the NTP Control Message Protocol via UDP socket, similar to `ntpq`, the [standard NTP query program](http://doc.ntp.org/current-stable/ntpq.html).

**Requirements:**
 * Version: `NTPv4`
 * Local interrogation allowed in `/etc/ntp.conf` (default):

```
# Local users may interrogate the ntp server more closely.
restrict 127.0.0.1
restrict ::1
```

It produces:

1. system
 * offset
 * jitter
 * frequency
 * delay
 * dispersion
 * stratum
 * tc
 * precision

2. peers
 * offset
 * delay
 * dispersion
 * jitter
 * rootdelay
 * rootdispersion
 * stratum
 * hmode
 * pmode
 * hpoll
 * ppoll
 * precision

**configuration**

Sample:

```yaml
update_every: 10

host: 'localhost'
port: '123'
show_peers: yes
# hide peers with source address in ranges 127.0.0.0/8 and 192.168.0.0/16
peer_filter: '(127\..*)|(192\.168\..*)'
# check for new/changed peers every 60 updates
peer_rescan: 60
```

Sample (multiple jobs):

Note: `ntp.conf` on the host `otherhost` must be configured to allow queries from our local host by including a line like `restrict <IP> nomodify notrap nopeer`.

```yaml
local:
    host: 'localhost'

otherhost:
    host: 'otherhost'
```

If no configuration is given, module will attempt to connect to `ntpd` on `::1:123` or `127.0.0.1:123` and show charts for the systemvars. Use `show_peers: yes` to also show the charts for configured peers. Local peers in the range `127.0.0.0/8` are hidden by default, use `peer_filter: ''` to show all peers.

---
