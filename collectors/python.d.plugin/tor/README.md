# tor

Monitors a local tor daemon.

Provides the following charts:

1. **Traffic**
 * read
 * write

### configuration

```yaml
local:
  # how to connect to the daemon
  # 'port': connect over TCP (default)
  # 'socket': use a unix socket
  connect_method: 'port'
  # password
  password: 'REPLACEME'
  # TCP port
  port: 9051
  # unix socket path
  socket_path: '/tmp/foo'
```

When the unix socket is used, the system user running python.d.plugin (usually named "netdata") needs
permissions to read and write to the socket.

---
