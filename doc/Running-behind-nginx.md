# netdata via nginx

To pass netdata via a nginx, use this:

### As a virtual host

```
upstream backend {
    # the netdata server
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

### As a subfolder for multiple netdata servers, via one nginx

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

Using the above, you access netdata on the backend servers, like this:

- `http://nginx.server/netdata/server1/` to reach `backend-server1`
- `http://nginx.server/netdata/server2/` to reach `backend-server2`


### Enable authentication

Create an authentication file to enable the nginx basic authentication.
Do not use authentication without SSL/TLS!
If you haven't one you can do the following:

```
printf "yourusername:$(openssl passwd -apr1)" > /etc/nginx/passwords
```

And enable the authentication inside your server directive:

```
server {
    # ...
    auth_basic "Protected";
    auth_basic_user_file passwords;
    # ...
}
```

## limit direct access to netdata

If your nginx is on `localhost`, you can use this to protect your netdata:

```
[web]
    bind to = 127.0.0.1 ::1
```

---

You can also use a unix domain socket. This will also provide a faster route between nginx and netdata:

```
[web]
    bind to = unix:/tmp/netdata.sock
```
_note: netdata v1.8+ support unix domain sockets_

At the nginx side, use something like this to use the same unix domain socket:

```
upstream backend {
    server unix:/tmp/netdata.sock;
    keepalive 64;
}
```

---

If your nginx server is not on localhost, you can set:

```
[web]
    bind to = *
    allow connections from = IP_OF_NGINX_SERVER
```

_note: netdata v1.9+ support `allow connections from`_

`allow connections from` accepts [netdata simple patterns](https://github.com/netdata/netdata/wiki/Configuration#netdata-simple-patterns) to match against the connection IP address.

## prevent the double access.log

nginx logs accesses and netdata logs them too. You can prevent netdata from generating its access log, by setting this in `/etc/netdata/netdata.conf`:

```
[global]
      access log = none
```

## SELinux

If you get an 502 Bad Gateway error you might check your nginx error log:

```sh
# cat /var/log/nginx/error.log:
2016/09/09 12:34:05 [crit] 5731#5731: *1 connect() to 127.0.0.1:19999 failed (13: Permission denied) while connecting to upstream, client: 1.2.3.4, server: netdata.example.com, request: "GET / HTTP/2.0", upstream: "http://127.0.0.1:19999/", host: "netdata.example.com"
```

If you see something like the above, chances are high that SELinux prevents nginx from connecting to the backend server. To fix that, just use this policy: `setsebool -P httpd_can_network_connect true`.
