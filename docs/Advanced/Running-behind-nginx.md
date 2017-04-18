# netdata via nginx

To pass netdata via a nginx, use this:

```
upstream backend {
    # the netdata server
    server 127.0.0.1:19999;
    keepalive 64;
}

server {
    # nginx listens to this
    listen 10.1.1.1:80;

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

### Access multiple netdata servers, via one nginx

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
    listen 10.1.1.1;
    server_name 10.1.1.1;

    location ~ /netdata/(?<behost>.*)/(?<ndpath>.*) {
        proxy_set_header X-Forwarded-Host $host;
        proxy_set_header X-Forwarded-Server $host;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_pass http://backend-$behost/$ndpath$is_args$args;
        proxy_http_version 1.1;
        proxy_pass_request_headers on;
        proxy_set_header Connection "keep-alive";
        proxy_store off;
    }
}
```

Of course you can add as many backend servers as you like.

Using the above, you access netdata on the backend servers, like this:

- `http://nginx.server/netdata/server1/` to reach `backend-server1`
- `http://nginx.server/netdata/server2/` to reach `backend-server2`


### Enable authentication

Create an authentication file to enable the nginx basic authentication. If you haven't one you can do the following:

```
printf "yourusername:$(openssl passwd -crypt 'yourpassword')" > /etc/nginx/passwords
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

You would also need to instruct netdata to listen only to `127.0.0.1` or `::1`.

To limit access to netdata only from localhost, set `bind socket to IP = 127.0.0.1` or `bind socket to IP = ::1` in `/etc/netdata/netdata.conf`.
