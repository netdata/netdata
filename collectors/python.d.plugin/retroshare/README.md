# retroshare

[RetroShare](https://retroshare.cc/) is a free and open-source peer-to-peer communication and file sharing app based on a friend-to-friend network.

This module will monitor one or more `RetroShare` applications, depending on your configuration.

## Charts

This module produces the following charts:

-   Bandwidth in `kilobits/s`
-   Peers in `peers`
-   DHT in `peers`


## Configuration

Here is an example for 2 servers:

```yaml
localhost:
  url      : 'http://localhost:9090'
  user     : "user"
  password : "pass"

remote:
  url      : 'http://203.0.113.1:9090'
  user     : "user"
  password : "pass"
```
---

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fcollectors%2Fpython.d.plugin%2Fretroshare%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
