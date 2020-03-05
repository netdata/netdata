# Exporting metrics to external databases (experimental)

The exporting engine is an update for the former [backends](../backends/). It's still work in progress. It has a
modular structure and supports metric exporting via multiple exporting connector instances at the same time. You can
have different update intervals and filters configured for every exporting connector instance. The exporting engine has
its own configuration file `exporting.conf`. Configuration is almost similar to [backends](../backends/#configuration).
The only difference is that the type of a connector should be specified in a section name before a colon and a name after
the colon. At the moment only four types of connectors are supported: `graphite`, `json`, `opentsdb`, `opentsdb:http`.

An example configuration:
```conf
[exporting:global]
enabled = yes

[graphite:my_instance1]
enabled = yes
destination = localhost:2003
data source = sum
update every = 5
send charts matching = system.load

[json:my_instance2]
enabled = yes
destination = localhost:5448
data source = as collected
update every = 2
send charts matching = system.active_processes

[opentsdb:my_instance3]
enabled = yes
destination = localhost:4242
data source = sum
update every = 10
send charts matching = system.cpu

[opentsdb:http:my_instance4]
enabled = yes
destination = localhost:4243
data source = average
update every = 3
send charts matching = system.active_processes

```

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fexporting%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
