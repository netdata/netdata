# spigotmc

This module does some really basic monitoring for Spigot Minecraft servers.

It provides two charts, one tracking server-side ticks-per-second in
1, 5 and 15 minute averages, and one tracking the number of currently
active users.

This is not compatible with Spigot plugins which change the format of
the data returned by the `tps` or `list` console commands.

## configuration

```yaml
host: localhost
port: 25575
password: pass
```

By default, a connection to port 25575 on the local system is attempted with an empty password.

---

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fcollectors%2Fpython.d.plugin%2Fspigotmc%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
