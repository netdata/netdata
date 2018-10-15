# spigotmc

This module does some really basic monitoring for Spigot Minecraft servers.

It provides two charts, one tracking server-side ticks-per-second in
1, 5 and 15 minute averages, and one tracking the number of currently
active users.

This is not compatible with Spigot plugins which change the format of
the data returned by the `tps` or `list` console commands.

### configuration

```yaml
host: localhost
port: 25575
password: pass
```

By default, a connection to port 25575 on the local system is attempted with an empty password.

---
