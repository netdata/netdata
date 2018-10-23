# memcached

Memcached monitoring module. Data grabbed from [stats interface](https://github.com/memcached/memcached/wiki/Commands#stats).

1. **Network** in kilobytes/s
 * read
 * written

2. **Connections** per second
 * current
 * rejected
 * total

3. **Items** in cluster
 * current
 * total

4. **Evicted and Reclaimed** items
 * evicted
 * reclaimed

5. **GET** requests/s
 * hits
 * misses

6. **GET rate** rate in requests/s
 * rate

7. **SET rate** rate in requests/s
 * rate

8. **DELETE** requests/s
 * hits
 * misses

9. **CAS** requests/s
 * hits
 * misses
 * bad value

10. **Increment** requests/s
 * hits
 * misses

11. **Decrement** requests/s
 * hits
 * misses

12. **Touch** requests/s
 * hits
 * misses

13. **Touch rate** rate in requests/s
 * rate

### configuration

Sample:

```yaml
localtcpip:
  name     : 'local'
  host     : '127.0.0.1'
  port     : 24242
```

If no configuration is given, module will attempt to connect to memcached instance on `127.0.0.1:11211` address.

---
