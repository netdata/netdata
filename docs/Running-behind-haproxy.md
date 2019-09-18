# Netdata via HAProxy

> HAProxy is a free, very fast and reliable solution offering high availability, load balancing, 
> and proxying for TCP and HTTP-based applications. It is particularly suited for very high traffic web sites 
> and powers quite a number of the world's most visited ones.

If Netdata is running on a host running HAProxy, rather than connecting to Netdata from a port number, a domain name 
can be pointed at HAProxy, and HAProxy can redirect connections to the Netdata port. This can make it possible to 
connect to Netdata at <https://example.com> or <https://example.com/netdata/>, which is a much nicer experience then <http://example.com:19999>.

To proxy requests from [HAProxy](https://github.com/haproxy/haproxy) to Netdata, 
the following configuration can be used:

## Default Configuration

For all examples, set the mode to `http`

```conf
defaults
    mode    http
```

## Simple Configuration

A simple example where the base URL, say <http://example.com>, is used with no subpath:

### Frontend

Create a frontend to recieve the request.

```conf
frontend http_frontend
    ## HTTP ipv4 and ipv6 on all ips ##
    bind :::80 v4v6

    default_backend     netdata_backend
```

### Backend

Create the Netdata backend which will send requests to port `19999`.

```conf
backend netdata_backend
    option       forwardfor
    server       netdata_local     127.0.0.1:19999

    http-request set-header Host %[src]
    http-request set-header X-Forwarded-For %[src]
    http-request set-header X-Forwarded-Port %[dst_port]
    http-request set-header Connection "keep-alive"
```

## Configuration with subpath

A example where the base URL is used with a subpath `/netdata/`:

### Frontend

To use a subpath, create an ACL, which will set a variable based on the subpath.

```conf
frontend http_frontend
    ## HTTP ipv4 and ipv6 on all ips ##
    bind :::80 v4v6

    # URL begins with /netdata
    acl is_netdata url_beg  /netdata

    # if trailing slash is missing, redirect to /netdata/
    http-request redirect scheme https drop-query append-slash if is_netdata ! { path_beg /netdata/ }

    ## Backends ##
    use_backend     netdata_backend       if is_netdata

    # Other requests go here (optional)
    # put netdata_backend here if no others are used
    default_backend www_backend
```

### Backend

Same as simple example, expept remove `/netdata/` with regex.

```conf
backend netdata_backend
    option      forwardfor
    server      netdata_local     127.0.0.1:19999

    http-request set-path %[path,regsub(^/netdata/,/)]

    http-request set-header Host %[src]
    http-request set-header X-Forwarded-For %[src]
    http-request set-header X-Forwarded-Port %[dst_port]
    http-request set-header Connection "keep-alive"
```

## Using TLS communication

TLS can be used by adding port `443` and a cert to the frontend. 
This example will only use Netdata if host matches example.com (replace with your domain).

### Frontend

This frontend uses a certificate list.

```conf
frontend https_frontend
    ## HTTP ##
    bind :::80 v4v6
    # Redirect all HTTP traffic to HTTPS with 301 redirect
    redirect scheme https code 301 if !{ ssl_fc }

    ## HTTPS ##
    # Bind to all v4/v6 addresses, use a list of certs in file
    bind :::443 v4v6 ssl crt-list /etc/letsencrypt/certslist.txt

    ## ACL ##
    # Optionally check host for Netdata
    acl is_example_host  hdr_sub(host) -i example.com

    ## Backends ##
    use_backend     netdata_backend       if is_example_host
    # Other requests go here (optional)
    default_backend www_backend
```

In the cert list file place a mapping from a certificate file to the domain used:

`/etc/letsencrypt/certslist.txt`:

```txt
example.com /etc/letsencrypt/live/example.com/example.com.pem
```

The file `/etc/letsencrypt/live/example.com/example.com.pem` should contain the key and 
certificate (in that order) concatenated into a `.pem` file.:

```sh
cat /etc/letsencrypt/live/example.com/fullchain.pem \
    /etc/letsencrypt/live/example.com/privkey.pem > \
    /etc/letsencrypt/live/example.com/example.com.pem
```

### Backend

Same as simple, except set protocol `https`.

```conf
backend netdata_backend
    option forwardfor
    server      netdata_local     127.0.0.1:19999

    http-request add-header X-Forwarded-Proto https
    http-request set-header Host %[src]
    http-request set-header X-Forwarded-For %[src]
    http-request set-header X-Forwarded-Port %[dst_port]
    http-request set-header Connection "keep-alive"
```

## Enable authentication

To use basic HTTP Authentication, create a authentication list:

```conf
# HTTP Auth
userlist basic-auth-list
  group is-admin
  # Plaintext password
  user admin password passwordhere groups is-admin
```

You can create a hashed password using the `mkpassword` utility.

```sh
 printf "passwordhere" | mkpasswd --stdin --method=sha-256
$5$l7Gk0VPIpKO$f5iEcxvjfdF11khw.utzSKqP7W.0oq8wX9nJwPLwzy1
```

Replace `passwordhere` with hash:

```conf
user admin password $5$l7Gk0VPIpKO$f5iEcxvjfdF11khw.utzSKqP7W.0oq8wX9nJwPLwzy1 groups is-admin
```

Now add at the top of the backend:

```conf
acl devops-auth http_auth_group(basic-auth-list) is-admin
http-request auth realm netdata_local unless devops-auth
```

## Full Example

Full example configuration with HTTP auth over TLS with subpath:

```conf
global
    maxconn     20000

    log         /dev/log local0
    log         /dev/log local1 notice
    user        haproxy
    group       haproxy
    pidfile     /run/haproxy.pid

    stats socket /run/haproxy/admin.sock mode 660 level admin expose-fd listeners
    stats timeout 30s
    daemon

    tune.ssl.default-dh-param 4096  # Max size of DHE key

    # Default ciphers to use on SSL-enabled listening sockets.
    ssl-default-bind-ciphers ECDH+AESGCM:DH+AESGCM:ECDH+AES256:DH+AES256:ECDH+AES128:DH+AES:RSA+AESGCM:RSA+AES:!aNULL:!MD5:!DSS
    ssl-default-bind-options no-sslv3

defaults
    log     global
    mode    http
    option  httplog
    option  dontlognull
    timeout connect 5000
    timeout client  50000
    timeout server  50000
    errorfile 400 /etc/haproxy/errors/400.http
    errorfile 403 /etc/haproxy/errors/403.http
    errorfile 408 /etc/haproxy/errors/408.http
    errorfile 500 /etc/haproxy/errors/500.http
    errorfile 502 /etc/haproxy/errors/502.http
    errorfile 503 /etc/haproxy/errors/503.http
    errorfile 504 /etc/haproxy/errors/504.http

frontend https_frontend
    ## HTTP ##
    bind :::80 v4v6
    # Redirect all HTTP traffic to HTTPS with 301 redirect
    redirect scheme https code 301 if !{ ssl_fc }

    ## HTTPS ##
    # Bind to all v4/v6 addresses, use a list of certs in file
    bind :::443 v4v6 ssl crt-list /etc/letsencrypt/certslist.txt

    ## ACL ##
    # Optionally check host for Netdata
    acl is_example_host  hdr_sub(host) -i example.com
    acl is_netdata       url_beg  /netdata

    http-request redirect scheme https drop-query append-slash if is_netdata ! { path_beg /netdata/ }

    ## Backends ##
    use_backend     netdata_backend       if is_example_host is_netdata
    default_backend www_backend

# HTTP Auth
userlist basic-auth-list
    group is-admin
    # Hashed password
    user admin password $5$l7Gk0VPIpKO$f5iEcxvjfdF11khw.utzSKqP7W.0oq8wX9nJwPLwzy1 groups is-admin

## Default server(s) (optional)##
backend www_backend
    mode        http
    balance     roundrobin
    timeout     connect 5s
    timeout     server  30s
    timeout     queue   30s

    http-request add-header 'X-Forwarded-Proto: https'
    server      other_server 111.111.111.111:80 check

backend netdata_backend
    acl devops-auth http_auth_group(basic-auth-list) is-admin
    http-request auth realm netdata_local unless devops-auth

    option forwardfor
    server      netdata_local     127.0.0.1:19999

    http-request set-path %[path,regsub(^/netdata/,/)]

    http-request add-header X-Forwarded-Proto https
    http-request set-header Host %[src]
    http-request set-header X-Forwarded-For %[src]
    http-request set-header X-Forwarded-Port %[dst_port]
    http-request set-header Connection "keep-alive"
```

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fdocs%2FRunning-behind-haproxy&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
