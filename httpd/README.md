# Netdata new H2O web server

We are in the process of replacing the internal Netdata custom webserver with one based on [libh2o](https://github.com/h2o/h2o). 
The new web server is disabled by default, while it is still in development.

h2o will enable us to use HTTP/2 and HTTP/3 in the future, with all the benefits these protocols provide. 
We will also be able to use HTTP/2 server push to send the dashboard to the browser before it requests it. 
Finally, we will be able to provide authentication and other highly requested features.

## Configuration

To enable the h2o web server, you need edit `netdata.conf` and set

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

By default it will use the same `ssl key` and `ssl certificate` you may have defined under the `[web]` section of `netdata.conf`. 
To change the path, set

```
[httpd]
    ssl key = /etc/ssl/private/netdata.key
    ssl certificate = /etc/ssl/certs/netdata.pem
```

When HTTPS (TLS) is enabled you need to use `https://` in the browser to access the dashboard. If you are using a self-signed certificate, you will need to accept the certificate in the browser. In TLS mode `http` is disabled for security reasons.


## HTTP/2 and HTTP/3

To be able to use HTTP/2 (to realize the full potential of libh2o) you need to use TLS. HTTP/2 is not supported in plain HTTP. HTTP/3 support is planned to be added with the next stable release of libh2o.

## Streaming

It is possible to stream from Child nodes to a Parent through libh2o (both plain and TLS). Currently both Children and the Parent have to be updated to do that. Older Children connecting to a new parent will have to connect trough an internal webserver. New Children connecting to an old parent will be supported.

