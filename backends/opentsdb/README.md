# OpenTSDB with HTTP

Since version 1.16 the Netdata has the feature to communicate with OpenTSDB using HTTP API. To enable this channel
it is necessary to set the following options in your netdata.conf

```
[backend]
    type = opentsdb:http
    destination = localhost:4242
```

, in this example we are considering that OpenTSDB is running with its default port (4242).

## HTTPS

Netdata also supports sending the metrics using SSL/TLS, this is the preferred option, because it is the safest, but OpenTDSB
does not have support to safety connections, so it will be necessary to configure a reverse-proxy to enable the HTTPS communication.
 In our tests we used Nginx as reverse-proxy, we followed the instructions in the link (https://gist.github.com/torkelo/901f534b8b29b5920ea9 ),
we configured our nginx with the following lines

```
 server {
    listen 8082;
    servername localhost;

    location / {
        if ($request_method = 'OPTIONS') {
          add_header 'Access-Control-Allow-Origin' "$http_origin";
          add_header 'Access-Control-Allow-Credentials' 'true';
          add_header 'Access-Control-Max-Age' 1728000;
          add_header 'Access-Control-Allow-Methods' 'GET, POST, OPTIONS';
          add_header 'Access-Control-Allow-Headers' 'Authorization,Content-Type,Accept,Origin,User-Agent,DNT,Cache-Control,X-Mx-ReqToken,Keep-Alive,X-Requested-With,If-Modified-Since';
          add_header 'Content-Length' 0;
          add_header 'Content-Type' 'text/plain charset=UTF-8';
          return 204;
        }

        proxy_pass                 http://127.0.0.1:4242;
        proxy_set_header           X-Real-IP   $remote_addr;
        proxy_set_header           X-Forwarded-For  $proxy_add_x_forwarded_for;
        proxy_set_header           X-Forwarded-Proto  $scheme;
        proxy_set_header           X-Forwarded-Server  $host;
        proxy_set_header           X-Forwarded-Host  $host;
        proxy_set_header           Host  $host;

        client_max_body_size       10m;
        client_body_buffer_size    128k;

        proxy_connect_timeout      90;
        proxy_send_timeout         90;
        proxy_read_timeout         90;

        proxy_buffer_size          4k;
        proxy_buffers              4 32k;
        proxy_busy_buffers_size    64k;
        proxy_temp_file_write_size 64k;
    }

    add_header Access-Control-Allow-Origin "*";
    add_header Access-Control-Allow-Methods "GET, OPTIONS";
    add_header Access-Control-Allow-Headers "origin, authorization, accept";
}
```

Now it is necessary to do the next changes in the netdata.conf to enable the HTTPS communication between Netdata and OpenTSDB
trough Nginx

```
[backend]
    type = opentsdb:https
    destination = localhost:8082
```
