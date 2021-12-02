<!--
title: "Web server"
description: "The Netdata Agent's local static-threaded web server serves dashboards and real-time visualizations with security and DDoS protection."
custom_edit_url: https://github.com/netdata/netdata/edit/master/web/server/README.md
-->

# Web server

The Netdata web server runs as `static-threaded`, i.e. with a fixed, configurable number of threads.
It uses non-blocking I/O and respects the `keep-alive` HTTP header to serve multiple HTTP requests via the same connection.

## Configuration

Disable the web server by editing `netdata.conf` and setting:

```
[web]
    mode = none
```

With the web server enabled, control the number of threads and sockets with the following settings:

```
[web]
    web server threads = 4
    web server max sockets = 512
```

The default number of processor threads is `min(cpu cores, 6)`.

The `web server max sockets` setting is automatically adjusted to 50% of the max number of open files Netdata is allowed to use (via `/etc/security/limits.conf` or systemd), to allow enough file descriptors to be available for data collection.

### Binding Netdata to multiple ports

Netdata can bind to multiple IPs and ports, offering access to different services on each. Up to 100 sockets can be used (increase it at compile time with `CFLAGS="-DMAX_LISTEN_FDS=200" ./netdata-installer.sh ...`).

The ports to bind are controlled via `[web].bind to`, like this:

```
[web]
   default port = 19999
   bind to = 127.0.0.1=dashboard^SSL=optional 10.1.1.1:19998=management|netdata.conf hostname:19997=badges [::]:19996=streaming^SSL=force localhost:19995=registry *:http=dashboard unix:/run/netdata/netdata.sock
```

Using the above, Netdata will bind to:

-   IPv4 127.0.0.1 at port 19999 (port was used from `default port`). Only the UI (dashboard) and the read API will be accessible on this port. Both HTTP and HTTPS requests will be accepted.
-   IPv4 10.1.1.1 at port 19998. The management API and `netdata.conf` will be accessible on this port.
-   All the IPs `hostname` resolves to (both IPv4 and IPv6 depending on the resolved IPs) at port 19997. Only badges will be accessible on this port.
-   All IPv6 IPs at port 19996. Only metric streaming requests from other Netdata agents will be accepted on this port. Only encrypted streams will be allowed (i.e. child nodes also need to be [configured for TLS](/streaming/README.md).
-   All the IPs `localhost` resolves to (both IPv4 and IPv6 depending the resolved IPs) at port 19996. This port will only accept registry API requests.
-   All IPv4 and IPv6 IPs at port `http` as set in `/etc/services`. Only the UI (dashboard) and the read API will be accessible on this port. 
-   Unix domain socket `/run/netdata/netdata.sock`. All requests are serviceable on this socket. Note that in some OSs like Fedora, every service sees a different `/tmp`, so don't create a Unix socket under `/tmp`. `/run` or `/var/run` is suggested.

The option `[web].default port` is used when an entries in `[web].bind to` do not specify a port.

Note that the access permissions specified with the `=request type|request type|...` format are available from version 1.12 onwards. 
As shown in the example above, these permissions are optional, with the default being to permit all request types on the specified port. 
The request types are strings identical to the `allow X from` directives of the access lists, i.e. `dashboard`, `streaming`, `registry`, `netdata.conf`, `badges` and `management`. 
The access lists themselves and the general setting `allow connections from` in the next section are applied regardless of the ports that are configured to provide these services. 
The API requests are serviced as follows:

-   `dashboard` gives access to the UI, the read API and badges API calls.
-   `badges` gives access only to the badges API calls.
-   `management` gives access only to the management API calls.

### Enabling TLS support

Since v1.16.0, Netdata supports encrypted HTTP connections to the web server, plus encryption of streaming data to a
parent from its child nodes, via the TLS protocol.

Inbound unix socket connections are unaffected, regardless of the TLS settings.

> While Netdata uses Transport Layer Security (TLS) 1.2 to encrypt communications rather than the obsolete SSL protocol,
> it's still common practice to refer to encrypted web connections as `SSL`. Many vendors, like Nginx and even Netdata
> itself, use `SSL` in configuration files, whereas documentation will always refer to encrypted communications as `TLS`
> or `TLS/SSL`.

To enable TLS, provide the path to your certificate and private key in the `[web]` section of `netdata.conf`:

```conf
[web]
	ssl key = /etc/netdata/ssl/key.pem
	ssl certificate = /etc/netdata/ssl/cert.pem
```

Both files must be readable by the `netdata` user. If either of these files do not exist or are unreadable, Netdata will fall back to HTTP. For a parent-child connection, only the parent needs these settings.

For test purposes, generate self-signed certificates with the following command:

```bash
openssl req -newkey rsa:2048 -nodes -sha512 -x509 -days 365 -keyout key.pem -out cert.pem
```

> If you use 4096 bits for your key and the certificate, Netdata will need more CPU to process the communication.
> `rsa4096` can be up to 4 times slower than `rsa2048`, so we recommend using 2048 bits. Verify the difference
> by running:
>
> ```sh
> openssl speed rsa2048 rsa4096
> ```

### Select TLS version

Beginning with version 1.21, specify the TLS version and the ciphers that you want to use:

```conf
[web]
    tls version = 1.3
    tls ciphers = TLS_AES_256_GCM_SHA384:TLS_CHACHA20_POLY1305_SHA256:TLS_AES_128_GCM_SHA256
```

If you do not specify these options, Netdata will use the highest available protocol version on your system and the default cipher list for that protocol provided by your TLS implementation.

While Netdata accepts all the TLS version as arguments (`1` or `1.0`, `1.1`, `1.2` and `1.3`), we recommend you use `1.3` for the most secure encryption.

#### TLS/SSL enforcement

When the certificates are defined and unless any other options are provided, a Netdata server will:

-   Redirect all incoming HTTP web server requests to HTTPS. Applies to the dashboard, the API, `netdata.conf` and badges.
-   Allow incoming child connections to use both unencrypted and encrypted communications for streaming.

To change this behavior, you need to modify the `bind to` setting in the `[web]` section of `netdata.conf`. At the end of each port definition, append `^SSL=force` or `^SSL=optional`. What happens with these settings differs, depending on whether the port is used for HTTP/S requests, or for streaming.

| SSL setting | HTTP requests|HTTPS requests|Unencrypted Streams|Encrypted Streams|
|:---------:|:-----------:|:------------:|:-----------------:|:----------------|
| none | Redirected to HTTPS|Accepted|Accepted|Accepted|
| `force`| Redirected to HTTPS|Accepted|Denied|Accepted|
| `optional`| Accepted|Accepted|Accepted|Accepted|

Example:

```
[web]
    bind to = *=dashboard|registry|badges|management|streaming|netdata.conf^SSL=force
```

For information how to configure the child to use TLS, check [securing the communication](/streaming/README.md#securing-streaming-communications) in the streaming documentation. There you will find additional details on the expected behavior for client and server nodes, when their respective TLS options are enabled.

When we define the use of SSL in a Netdata agent for different ports,  Netdata will apply the behavior specified on each port. For example, using the configuration line below:

```
[web]
    bind to = *=dashboard|registry|badges|management|streaming|netdata.conf^SSL=force *:20000=netdata.conf^SSL=optional *:20001=dashboard|registry
```

Netdata will:

-   Force all HTTP requests to the default port to be redirected to HTTPS (same port).
-   Refuse unencrypted streaming connections from child nodes on the default port.
-   Allow both HTTP and HTTPS requests to port 20000 for `netdata.conf`
-   Force HTTP requests to port 20001 to be redirected to HTTPS (same port). Only allow requests for the dashboard, the read API and the registry on port 20001.

#### TLS/SSL errors

When you start using Netdata with TLS, you may find errors in the Netdata log, which is stored at `/var/log/netdata/error.log` by default.

Most of the time, these errors are due to incompatibilities between your browser's options related to TLS/SSL protocols and Netdata's internal configuration. The most common error is `error:00000006:lib(0):func(0):EVP lib`. 

In the near future, Netdata will allow our users to change the internal configuration to avoid similar errors. Until then, we're recommending only the most common and safe encryption protocols listed above.

### Access lists

Netdata supports access lists in `netdata.conf`:

```
[web]
	allow connections from = localhost *
	allow dashboard from = localhost *
	allow badges from = *
	allow streaming from = *
	allow netdata.conf from = localhost fd* 10.* 192.168.* 172.16.* 172.17.* 172.18.* 172.19.* 172.20.* 172.21.* 172.22.* 172.23.* 172.24.* 172.25.* 172.26.* 172.27.* 172.28.* 172.29.* 172.30.* 172.31.*
	allow management from = localhost
```

`*` does string matches on the IPs or FQDNs of the clients.

-   `allow connections from` matches anyone that connects on the Netdata port(s).
     So, if someone is not allowed, it will be connected and disconnected immediately, without reading even
     a single byte from its connection. This is a global settings with higher priority to any of the ones below.

-   `allow dashboard from` receives the request and examines if it is a static dashboard file or an API call the
     dashboards do.

-   `allow badges from` checks if the API request is for a badge. Badges are not matched by `allow dashboard from`.

-   `allow streaming from` checks if the child willing to stream metrics to this Netdata is allowed.
     This can be controlled per API KEY and MACHINE GUID in `stream.conf`.
     The setting in `netdata.conf` is checked before the ones in `stream.conf`.

-   `allow netdata.conf from` checks the IP to allow `http://netdata.host:19999/netdata.conf`.
     The IPs listed are all the private IPv4 addresses, including link local IPv6 addresses. Keep in mind that connections to Netdata API ports are filtered by `allow connections from`. So, IPs allowed by `allow netdata.conf from` should also be allowed by `allow connections from`.

-   `allow management from` checks the IPs to allow API management calls. Management via the API is currently supported for [health](/web/api/health/README.md#health-management-api)

In order to check the FQDN of the connection without opening the Netdata agent to DNS-spoofing, a reverse-dns record
must be setup for the connecting host. At connection time the reverse-dns of the peer IP address is resolved, and
a forward DNS resolution is made to validate the IP address against the name-pattern.

Please note that this process can be expensive on a machine that is serving many connections. Each access list has an
associated configuration option to turn off DNS-based patterns completely to avoid incurring this cost at run-time:

```
	allow connections by dns = heuristic
	allow dashboard by dns = heuristic
	allow badges by dns = heuristic
	allow streaming by dns = heuristic
	allow netdata.conf by dns = no
	allow management by dns = heuristic
```

The three possible values for each of these options are `yes`, `no` and `heuristic`. The `heuristic` option disables
the check when the pattern only contains IPv4/IPv6 addresses or `localhost`, and enables it when wildcards are
present that may match DNS FQDNs.

### Other netdata.conf [web] section options

|setting|default|info|
|:-----:|:-----:|:---|
|ses max window|`15`|See [single exponential smoothing](/web/api/queries/des/README.md)|
|des max window|`15`|See [double exponential smoothing](/web/api/queries/des/README.md)|
|listen backlog|`4096`|The port backlog. Check `man 2 listen`.|
|web files owner|`netdata`|The user that owns the web static files. Netdata will refuse to serve a file that is not owned by this user, even if it has read access to that file. If the user given is not found, Netdata will only serve files owned by user given in `run as user`.|
|web files group|`netdata`|If this is set, Netdata will check if the file is owned by this group and refuse to serve the file if it's not.|
|disconnect idle clients after seconds|`60`|The time in seconds to disconnect web clients after being totally idle.|
|timeout for first request|`60`|How long to wait for a client to send a request before closing the socket. Prevents slow request attacks.|
|accept a streaming request every seconds|`0`|Can be used to set a limit on how often a parent node will accept streaming requests from child nodes in a [streaming and replication setup](/streaming/README.md)|
|respect do not track policy|`no`|If set to `yes`, Netdata will respect the user's browser preferences for [Do Not Track](https://www.eff.org/issues/do-not-track) (DNT) and storing cookies. If DNT is _enabled_ in the browser, and this option is set to `yes`, users will not be able to sign in to Netdata Cloud via their local Agent dashboard, and their node will not connect to any [registry](/registry/README.md). For certain browsers, users must disable DNT and change this option to `yes` for full functionality.|
|x-frame-options response header||[Avoid clickjacking attacks, by ensuring that the content is not embedded into other sites](https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/X-Frame-Options).|
|enable gzip compression|`yes`|When set to `yes`, Netdata web responses will be GZIP compressed, if the web client accepts such responses.|
|gzip compression strategy|`default`|Valid strategies are `default`, `filtered`, `huffman only`, `rle` and `fixed`|
|gzip compression level|`3`|Valid levels are 1 (fastest) to 9 (best ratio)|

## DDoS protection

If you publish your Netdata to the internet, you may want to apply some protection against DDoS:

1.  Use the `static-threaded` web server (it is the default)
2.  Use reasonable `[web].web server max sockets` (the default is)
3.  Don't use all your CPU cores for Netdata (lower `[web].web server threads`)
4.  Run the `netdata` process with a low process scheduling priority (the default is the lowest)
5.  If possible, proxy Netdata via a full featured web server (nginx, apache, etc)

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fweb%2Fserver%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
