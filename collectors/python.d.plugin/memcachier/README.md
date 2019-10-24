# memcachier

Memcachier monitoring module. Data is retrieved using the [analytics API](https://www.memcachier.com/documentation/analytics-api).
Based on the [Memcached plugin](../memcached).

This plugin expects to receive only one cache_id and only one cache_server per job instance.

If you're having issues, running the plugin in [debug mode](../#how-to-debug-a-python-module) will show the results from each request.

## Configuration

It is necessary to provide a valid user/pass parameter in the configuration file before using this plugin.
Due to this restriction, the plugin is deactivated by default.

Sample:

```
instance1:
  name: 'testing'
  user: '012DEF'
  pass: '32D1G175000000000000000000000000'
```

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fcollectors%2Fpython.d.plugin%2Fmemcachier%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)