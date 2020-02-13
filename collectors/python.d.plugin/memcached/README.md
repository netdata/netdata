# Memcached monitoring with Netdata

Collects memory-caching system performance metrics. It reads server response to stats command ([stats interface](https://github.com/memcached/memcached/wiki/Commands#stats)).


1.  **Network** in kilobytes/s

    -   read
    -   written

2.  **Connections** per second

    -   current
    -   rejected
    -   total

3.  **Items** in cluster

    -   current
    -   total

4.  **Evicted and Reclaimed** items

    -   evicted
    -   reclaimed

5.  **GET** requests/s

    -   hits
    -   misses

6.  **GET rate** rate in requests/s

    -   rate

7.  **SET rate** rate in requests/s

    -   rate

8.  **DELETE** requests/s

    -   hits
    -   misses

9.  **CAS** requests/s

    -   hits
    -   misses
    -   bad value

10. **Increment** requests/s

    -   hits
    -   misses

11. **Decrement** requests/s

    -   hits
    -   misses

12. **Touch** requests/s

    -   hits
    -   misses

13. **Touch rate** rate in requests/s

    -   rate

## Configuration

Edit the `python.d/memcached.conf` configuration file using `edit-config` from the your agent's [config
directory](../../../docs/step-by-step/step-04.md#find-your-netdataconf-file), which is typically at `/etc/netdata`.

```bash
cd /etc/netdata   # Replace this path with your Netdata config directory, if different
sudo ./edit-config python.d/memcached.conf
```

Sample:

```yaml
localtcpip:
  name     : 'local'
  host     : '127.0.0.1'
  port     : 24242
```

If no configuration is given, module will attempt to connect to memcached instance on `127.0.0.1:11211` address.

---

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fcollectors%2Fpython.d.plugin%2Fmemcached%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
