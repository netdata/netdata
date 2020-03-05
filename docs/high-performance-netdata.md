# High performance Netdata

If you plan to run a Netdata public on the internet, you will get the most performance out of it by following these
rules:

## 1. run behind nginx

The internal web server is optimized to provide the best experience with few clients connected to it. Normally a web
browser will make 4-6 concurrent connections to a web server, so that it can send requests in parallel. To best serve a
single client, Netdata spawns a thread for each connection it receives (so 4-6 threads per connected web browser).

If you plan to have your Netdata public on the internet, this strategy wastes resources. It provides a lock-free
environment so each thread is autonomous to serve the browser, but it does not scale well. Running Netdata behind nginx,
idle connections to Netdata can be reused, thus improving significantly the performance of Netdata.

In the following nginx configuration we do the following:

-   allow nginx to maintain up to 1024 idle connections to Netdata (so Netdata will have up to 1024 threads waiting for
    requests)
-   allow nginx to compress the responses of Netdata (later we will disable gzip compression at Netdata)
-   we disable wordpress pingback attacks and allow only GET, HEAD and OPTIONS requests.

```conf
upstream backend {
    server 127.0.0.1:19999;
    keepalive 1024;
}

server {
    listen *:80;
    server_name my.web.server.name;

    location / {
        proxy_set_header X-Forwarded-Host $host;
        proxy_set_header X-Forwarded-Server $host;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_pass http://backend;
        proxy_http_version 1.1;
        proxy_pass_request_headers on;
        proxy_set_header Connection "keep-alive";
        proxy_store off;
        gzip on;
        gzip_proxied any;
        gzip_types *;
        
        # Block any HTTP requests other than GET, HEAD, and OPTIONS
        limit_except GET HEAD OPTIONS {
            deny all;
        }
    }

    # WordPress Pingback Request Denial
    if ($http_user_agent ~* "WordPress") {
        return 403;
    }

}
```

Then edit `/etc/netdata/netdata.conf` and set these config options:

```conf
[global]
    bind socket to IP = 127.0.0.1
    access log = none
    disconnect idle web clients after seconds = 3600
    enable web responses gzip compression = no
```

These options:

-   `[global].bind socket to IP = 127.0.0.1` makes Netdata listen only for requests from localhost (nginx).
-   `[global].access log = none` disables the access.log of Netdata. It is not needed since Netdata only listens for
    requests on 127.0.0.1 and thus only nginx can access it. nginx has its own access.log for your record.
-   `[global].disconnect idle web clients after seconds = 3600` will kill inactive web threads after an hour of
    inactivity.
-   `[global].enable web responses gzip compression = no` disables gzip compression at Netdata (nginx will compress the
    responses).

## 2. increase open files limit (non-systemd)

By default Linux limits open file descriptors per process to 1024. This means that less than half of this number of
client connections can be accepted by both nginx and Netdata. To increase them, create 2 new files:

1.  `/etc/security/limits.d/nginx.conf`, with these contents:

```conf
nginx   soft    nofile  10000
nginx   hard    nofile  30000
```

2.  `/etc/security/limits.d/netdata.conf`, with these contents:

```conf
netdata   soft    nofile  10000
netdata   hard    nofile  30000
```

and to activate them, run:

```sh
sysctl -p
```

## 2b. increase open files limit (systemd)

Thanks to [@leleobhz](https://github.com/netdata/netdata/issues/655#issue-163932584), this is what you need to raise the
limits using systemd:

This is based on <https://ma.ttias.be/increase-open-files-limit-in-mariadb-on-centos-7-with-systemd/> and here worked as
following:

1.  Create the folders in /etc:

```bash
mkdir -p /etc/systemd/system/netdata.service.d
mkdir -p /etc/systemd/system/nginx.service.d
```

2.  Create limits.conf in each folder as following:

```conf
[Service]
LimitNOFILE=30000
```

3.  Reload systemd daemon list and restart services:

```sh
systemctl daemon-reload
systemctl restart netdata.service
systemctl restart nginx.service
```

You can check limits with following commands:

```sh
cat /proc/$(ps aux | grep "nginx: master process" | grep -v grep | awk '{print $2}')/limits | grep "Max open files"
cat /proc/$(ps aux | grep "\/[n]etdata " | head -n1 | grep -v grep | awk '{print $2}')/limits | grep "Max open files"
```

View of the files:

```sh
# tree /etc/systemd/system/*service.d/etc/systemd/system/netdata.service.d
/etc/systemd/system/netdata.service.d
└── limits.conf
/etc/systemd/system/nginx.service.d
└── limits.conf

0 directories, 2 files

# cat /proc/$(ps aux | grep "nginx: master process" | grep -v grep | awk '{print $2}')/limits | grep "Max open files"
Max open files            30000                30000                files     

# cat /proc/$(ps aux | grep "netdata" | head -n1 | grep -v grep | awk '{print $2}')/limits | grep "Max open files"
Max open files            30000                30000                files     
```

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fdocs%2Fhigh-performance-netdata&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
