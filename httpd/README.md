# Netdata new H2O web server

We are in process of replacing internal Netdata custom webserver with one based on [libh2o](https://github.com/h2o/h2o). As it is still in development, it is **disabled by default** (can be considered a technical preview for now).

h2o will enable us to use HTTP/2 and HTTP/3 (in future) with all benefits they provide. It will also allow us to use HTTP/2 server push to send the dashboard to the browser before it requests it. It will also allow us to implement authentication and other highly requested features.

## Configuration

To enable it, you need to edit `netdata.conf` and set

```
[httpd]
    enabled = yes
```

**by default** it will **listen on port 19998** to run alongside the default internal Netdata web server (running on 19999 by default).

To change the port, edit `netdata.conf` and set

```
[httpd]
    port = 19998
```

To enable TLS (HTTPS) edit `netdata.conf` and set

```
[httpd]
    ssl = yes
```

By default it will use the certificate and key from default path same as internal webserver. To change the path, edit `netdata.conf` and set

```
[httpd]
    ssl key = /etc/ssl/private/netdata.key
    ssl certificate = /etc/ssl/certs/netdata.pem
```

When HTTPS (TLS) is enabled you need to use `https://` in the browser to access the dashboard. If you are using a self-signed certificate, you will need to accept the certificate in the browser. In TLS mode `http` is disabled for security reasons.


## HTTP/2 and HTTP/3

To be able to use HTTP/2 (to realize full potential of libh2o) you need to use TLS. HTTP/2 are not supported in plain HTTP. HTTP/3 support is planned to be added with the next stable release of libh2o.

## Streaming

It is possible to stream from Child nodes to Parent trough libh2o (both plain and TLS). Currently both Child and Parent have to be updated to do that. Older Children connecting to new parent will have to connect trough internal webserver. New Children connection to old parent will be supported.

