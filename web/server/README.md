# Web server

Netdata supports 3 implementations of its internal web server:

- `static-threaded` is a web server with a fix (configured number of threads)
- `single-threaded` is a simple web server running with a single thread
- `multi-threaded` is a web server that spawns a thread for each client connection
- `none` to disable the web server

We suggest to use the `static-threaded` one. It is the most efficient.

All versions of the web servers use non-blocking I/O.

All web servers respect the `keep-alive` HTTP header to serve multiple HTTP requests via the same connection.

## Configuration

### Selecting the web server

You can select the web server implementation by editing `netdata.conf` and setting:

```
[web]
    mode = none | single-threaded | multi-threaded | static-threaded
```

The `static` web server supports also these settings:

```
[web]
    mode = static-threaded
    web server threads = 4
    web server max sockets = 512
```

The default number of processor threads is `min(cpu cores, 6)`.

The `web server max sockets` setting is automatically adjusted to 50% of the max number of open files netdata is allowed to use (via `/etc/security/limits.conf` or systemd), to allow enough file descriptors to be available for data collection.

### Binding netdata to multiple ports

Netdata can bind to multiple IPs and ports. Up to 100 sockets can be used (you can increase it at compile time with `CFLAGS="-DMAX_LISTEN_FDS=200" ./netdata-installer.sh ...`).

The ports to bind are controlled via `[web].bind to`, like this:

```
[web]
   default port = 19999
   bind to = 127.0.0.1 10.1.1.1:19998 hostname:19997 [::]:19996 localhost:19995 *:http unix:/tmp/netdata.sock
```

Using the above, netdata will bind to:

- IPv4 127.0.0.1 at port 19999 (port was used from `default port`)
- IPv4 10.1.1.1 at port 19998
- All the IPs `hostname` resolves to (both IPv4 and IPv6 depending on the resolved IPs) at port 19997
- All IPv6 IPs at port 19996
- All the IPs `localhost` resolves to (both IPv4 and IPv6 depending the resolved IPs) at port 19996
- All IPv4 and IPv6 IPs at port `http` as set in `/etc/services`
- Unix domain socket `/tmp/netdata.sock`

The option `[web].default port` is used when an entries in `[web].bind to` do not specify a port.

### Access lists

Netdata supports access lists in `netdata.conf`:

```
[web]
	allow connections from = localhost *
	allow dashboard from = localhost *
	allow badges from = *
	allow streaming from = *
	allow netdata.conf from = localhost fd* 10.* 192.168.* 172.16.* 172.17.* 172.18.* 172.19.* 172.20.* 172.21.* 172.22.* 172.23.* 172.24.* 172.25.* 172.26.* 172.27.* 172.28.* 172.29.* 172.30.* 172.31.*
```

`*` does string matches on the IPs of the clients.

- `allow connections from` matches anyone that connects on the netdata port(s).
   So, if someone is not allowed, it will be connected and disconnected immediately, without reading even
   a single byte from its connection. This is a global settings with higher priority to any of the ones below.

- `allow dashboard from` receives the request and examines if it is a static dashboard file or an API call the
   dashboards do.

- `allow badges from` checks if the API request is for a badge. Badges are not matched by `allow dashboard from`.

- `allow streaming from` checks if the slave willing to stream metrics to this netdata is allowed.
   This can be controlled per API KEY and MACHINE GUID in [stream.conf](../../streaming/stream.conf).
   The setting in `netdata.conf` is checked before the ones in [stream.conf](../../streaming/stream.conf).

- `allow netdata.conf from` checks the IP to allow `http://netdata.host:19999/netdata.conf`.
   The IPs listed are all the private IPv4 addresses, including link local IPv6 addresses. Keep in mind that connections to netdata API ports are filtered by `allow connections from`. So, IPs allowed by `allow netdata.conf from` should also be allowed by `allow connections from`.

### Other netdata.conf [web] section options
setting | default | info
:------:|:-------:|:----
ses max window | `15` | See [single exponential smoothing](../api/queries/des/)
des max window | `15` | See [double exponential smoothing](../api/queries/des/)
listen backlog | `4096` | The port backlog. Check `man 2 listen`.  
web files owner | `netdata` | The user that owns the web static files. Netdata will refuse to serve a file that is not owned by this user, even if it has read access to that file. If the user given is not found, netdata will only serve files owned by user given in `run as user`. 
web files group | `netdata` | If this is set, Netdata will check if the file is owned by this group and refuse to serve the file if it's not.
disconnect idle clients after seconds | `60` | The time in seconds to disconnect web clients after being totally idle. 
timeout for first request | `60` | How long to wait for a client to send a request before closing the socket. Prevents slow request attacks.
accept a streaming request every seconds | `0` | Can be used to set a limit on how often a master Netdata server will accept streaming requests from the slaves in a [streaming and replication setup](../../streaming)
respect do not track policy | `no` | If set to `yes`, will respect the client's browser preferences on storing cookies. 
x-frame-options response header |  | [Avoid clickjacking attacks, by ensuring that the content is not embedded into other sites](https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/X-Frame-Options).
enable gzip compression | `yes` | When set to `yes`, netdata web responses will be GZIP compressed, if the web client accepts such responses. 
gzip compression strategy | `default` | Valid strategies are `default`, `filtered`, `huffman only`, `rle` and `fixed`
gzip compression level | `3` | Valid levels are 1 (fastest) to 9 (best ratio)


## DDoS protection

If you publish your netdata to the internet, you may want to apply some protection against DDoS:

1. Use the `static-threaded` web server (it is the default)
2. Use reasonable `[web].web server max sockets` (the default is)
3. Don't use all your cpu cores for netdata (lower `[web].web server threads`)
4. Run netdata with a low process scheduling priority (the default is the lowest)
5. If possible, proxy netdata via a full featured web server (nginx, apache, etc)

