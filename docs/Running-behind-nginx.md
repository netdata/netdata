# Running Netdata behind Nginx

## Intro

[Nginx](https://nginx.org/en/) is an HTTP and reverse proxy server, a mail proxy server, and a generic TCP/UDP proxy server used to host websites and applications of all sizes. 

The software is known for its low impact on memory resources, high scalability, and its modular, event-driven architecture which can offer secure, predictable performance.

## Why Nginx

- By default, Nginx is fast and lightweight out of the box.

- Nginx is used and useful in cases when you want to access different instances of Netdata from a single server.

- Password-protect access to Netdata, until distributed authentication is implemented via the Netdata cloud Sign In mechanism.

- A proxy was necessary to encrypt the communication to Netdata, until v1.16.0, which provided TLS (HTTPS) support.

## Nginx configuration file

All Nginx configurations can be found in the `/etc/nginx/` directory. The main configuration file is `/etc/nginx/nginx.conf`. Website or app-specific configurations can be found in the `/etc/nginx/site-available/` directory.

Configuration options in Nginx are known as directives. Directives are organized into groups known as blocks or contexts. The two terms can be used interchangeably.

Depending on your installation source, you’ll find an example configuration file at `/etc/nginx/conf.d/default.conf` or `etc/nginx/sites-enabled/default`, in some cases you may have to manually create the `sites-available` and `sites-enabled` directories. 

You can edit the Nginx configuration file with Nano, Vim or any other text editors you are comfortable with.

After making changes to the configuration files:

- Test Nginx configuration with `nginx -t`.

- Restart Nginx to effect the change with `/etc/init.d/nginx restart` or `service nginx restart`.

## Ways to access Netdata via Nginx

### As a virtual host

With this method instead of `SERVER_IP_ADDRESS:19999`, the Netdata dashboard can be accessed via a human-readable URL such as `netdata.example.com` used in the configuration below. 

```
upstream backend {
    # the Netdata server
    server 127.0.0.1:19999;
    keepalive 64;
}

server {
    # nginx listens to this
    listen 80;

    # the virtual host name of this
    server_name netdata.example.com;

    location / {
        proxy_set_header X-Forwarded-Host $host;
        proxy_set_header X-Forwarded-Server $host;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_pass http://backend;
        proxy_http_version 1.1;
        proxy_pass_request_headers on;
        proxy_set_header Connection "keep-alive";
        proxy_store off;
    }
}
```
### As a subfolder to an existing virtual host

This method is recommended when Netdata is to be served from a subfolder (or directory). 
In this case, the virtual host `netdata.example.com` already exists and Netdata has to be accessed via `netdata.example.com/netdata/`.

```
upstream netdata {
    server 127.0.0.1:19999;
    keepalive 64;
}

server {
   listen 80;

   # the virtual host name of this subfolder should be exposed
   #server_name netdata.example.com;

   location = /netdata {
        return 301 /netdata/;
   }

   location ~ /netdata/(?<ndpath>.*) {
        proxy_redirect off;
        proxy_set_header Host $host;

        proxy_set_header X-Forwarded-Host $host;
        proxy_set_header X-Forwarded-Server $host;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_http_version 1.1;
        proxy_pass_request_headers on;
        proxy_set_header Connection "keep-alive";
        proxy_store off;
        proxy_pass http://netdata/$ndpath$is_args$args;

        gzip on;
        gzip_proxied any;
        gzip_types *;
    }
}
```

### As a subfolder for multiple Netdata servers, via one Nginx

This is the recommended configuration when one Nginx will be used to manage multiple Netdata servers via subfolders. 

```
upstream backend-server1 {
    server 10.1.1.103:19999;
    keepalive 64;
}
upstream backend-server2 {
    server 10.1.1.104:19999;
    keepalive 64;
}

server {
    listen 80;

    # the virtual host name of this subfolder should be exposed
    #server_name netdata.example.com;

    location ~ /netdata/(?<behost>.*)/(?<ndpath>.*) {
        proxy_set_header X-Forwarded-Host $host;
        proxy_set_header X-Forwarded-Server $host;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_http_version 1.1;
        proxy_pass_request_headers on;
        proxy_set_header Connection "keep-alive";
        proxy_store off;
        proxy_pass http://backend-$behost/$ndpath$is_args$args;

        gzip on;
        gzip_proxied any;
        gzip_types *;
    }

    # make sure there is a trailing slash at the browser
    # or the URLs will be wrong
    location ~ /netdata/(?<behost>.*) {
        return 301 /netdata/$behost/;
    }
}
```

Of course you can add as many backend servers as you like.

Using the above, you access Netdata on the backend servers, like this:

- `http://netdata.example.com/netdata/server1/` to reach `backend-server1`
- `http://netdata.example.com/netdata/server2/` to reach `backend-server2`

### Encrypt the communication between Nginx and Netdata

In case Netdata's web server has been [configured to use TLS](../web/server/#enabling-tls-support), it is necessary to specify inside the Nginx configuration that the final destination is using TLS. To do this, please, append the following parameters in your `nginx.conf`

```
proxy_set_header X-Forwarded-Proto https;
proxy_pass https://localhost:19999;
```

Optionally it is also possible to [enable TLS/SSL on Nginx](http://nginx.org/en/docs/http/configuring_https_servers.html), this way the user will encrypt not only the communication between Nginx and Netdata but also between the user and Nginx.

If Nginx is not configured as described here, you will probably receive the error `SSL_ERROR_RX_RECORD_TOO_LONG`.

### Enable authentication

Create an authentication file to enable basic authentication via Nginx, this secures your Netdata dashboard.

If you don't have an authentication file, you can use the following command:

```
printf "yourusername:$(openssl passwd -apr1)" > /etc/nginx/passwords
```

And then enable the authentication inside your server directive:

```
server {
    # ...
    auth_basic "Protected";
    auth_basic_user_file passwords;
    # ...
}
```

## Limit direct access to Netdata

If your Nginx is on `localhost`, you can use this to protect your Netdata:

```
[web]
    bind to = 127.0.0.1 ::1
```

---

You can also use a unix domain socket. This will also provide a faster route between Nginx and Netdata:

```
[web]
    bind to = unix:/tmp/netdata.sock
```
_note: Netdata v1.8+ support unix domain sockets_

At the Nginx side, use something like this to use the same unix domain socket:

```
upstream backend {
    server unix:/tmp/netdata.sock;
    keepalive 64;
}
```

---

If your Nginx server is not on localhost, you can set:

```
[web]
    bind to = *
    allow connections from = IP_OF_NGINX_SERVER
```

_note: Netdata v1.9+ support `allow connections from`_

`allow connections from` accepts [Netdata simple patterns](../libnetdata/simple_pattern/) to match against the connection IP address.

## Prevent the double access.log

Nginx logs accesses and Netdata logs them too. You can prevent Netdata from generating its access log, by setting this in `/etc/netdata/netdata.conf`:

```
[global]
      access log = none
```

## SELinux

If you get an 502 Bad Gateway error you might check your Nginx error log:

```sh
# cat /var/log/nginx/error.log:
2016/09/09 12:34:05 [crit] 5731#5731: *1 connect() to 127.0.0.1:19999 failed (13: Permission denied) while connecting to upstream, client: 1.2.3.4, server: netdata.example.com, request: "GET / HTTP/2.0", upstream: "http://127.0.0.1:19999/", host: "netdata.example.com"
```

If you see something like the above, chances are high that SELinux prevents nginx from connecting to the backend server. To fix that, just use this policy: `setsebool -P httpd_can_network_connect true`.


[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fdocs%2FRunning-behind-nginx&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)]()