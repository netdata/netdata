# tor

Module connects to tor control port to collect traffic statistics.

**Requirements:**
* `tor` program
* `stem` python package

It produces only one chart:

1. **Traffic**
 * read
 * write

### configuration

Needs `control_port` and `password`

Here is an example for local server:

```yaml
update_every : 1
priority     : 60000

local_tcp:
 name: 'local'
 control_port: 9051
 password: 'password'

local_socket:
 name: 'local'
 control_port: '/var/run/tor/control'
 password: 'password'
```

### prerequisite

Add to `/etc/tor/torrc`:

```
ControlPort 9051
```

For more options please read the manual.

Without configuration, module attempts to connect to `127.0.0.1:9051`.

---
