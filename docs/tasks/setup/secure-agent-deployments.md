<!--
title: "Secure Agent Netdata deployments"
sidebar_label: "Secure Netdata deployments"
custom_edit_url: "https://github.com/netdata/netdata/blob/master/docs/tasks/security/secure-netdata-deployments.md"
sidebar_position : 50
learn_status: "Published"
learn_topic_type: "Tasks"
learn_rel_path: "Setup"
learn_docs_purpose: "Present all the Agent interconnection ports and Cloud IPs for the user to whitelist them and hardening it"
-->

## Intro

The Netdata Agent started as a standalone deployment. A lightweight Web server serves the legacy dashboard and the API
at port `19999` of every Agent deployment. Although Netdata is read-only and runs without special privileges, if left
accessible to the internet at large, the local dashboard could reveal sensitive information about your infrastructure
so, we advise you to secure those endpoints. To do that, we give you many options to establish security best practices
that align with your goals and your organization's standards.

1. Restrict access via the Agent Web server's rules/options (Recommended)
2. Reverse proxy this Web Server via a Web server and secure in it's end (advanced users).
3. Secure Netdata Agent's ports via your firewall rules (advanced users)

## Restrict access via the Agent Web server's rules/options

We advise you to leave at least access to from your `localhost`, in any case it's up to you.

#### Steps:


```conf
[web]
    # Allow only localhost connections
    allow connections from = localhost

    # Allow only from management LAN running on `10.X.X.X`
    allow connections from = 10.*

    # Allow connections only from a specific FQDN/hostname
    allow connections from = example*
```

Note-internal:: Corner case streaming, you need to allow from the streaming source
The `allow connections from` setting is global and restricts access to the dashboard, badges, streaming, API, and
`netdata.conf`, but you can also set each of those access lists more granularly if you choose:

```conf
[web]
    allow connections from = localhost *
    allow dashboard from = localhost *
    allow badges from = *
    allow streaming from = *
    allow netdata.conf from = localhost fd* 10.* 192.168.* 172.16.* 172.17.* 172.18.* 172.19.* 172.20.* 172.21.* 172.22.* 172.23.* 172.24.* 172.25.* 172.26.* 172.27.* 172.28.* 172.29.* 172.30.* 172.31.*
    allow management from = localhost
```



Despite this design decision, your [data](/docs/netdata-security.md#your-data-is-safe-with-netdata) and your
[systems](/docs/netdata-security.md#your-systems-are-safe-with-netdata) are safe with Netdata. Netdata is read-only,
cannot do anything other than present metrics, and runs without special/`sudo` privileges. Also, the local dashboard
only exposes chart metadata and metric values, not raw data.

While Netdata is secure by design, we believe you
should [protect your nodes](/docs/netdata-security.md#why-netdata-should-be-protected). If left accessible to the
internet at large, the local dashboard could reveal sensitive information about your infrastructure. For example, an
attacker can view which applications you run (databases, webservers, and so on), or see every user account on a node.

Instead of dictating how to secure your infrastructure, 

- [Disable the local dashboard](#disable-the-local-dashboard): **Simplest and recommended method** for those who have
  added nodes to Netdata Cloud and view dashboards and metrics there.
- [Restrict access to the local dashboard](#restrict-access-to-the-local-dashboard): Allow local dashboard access from
  only certain IP addresses, such as a trusted static IP or connections from behind a management LAN. Full support for
  Netdata Cloud.
- [Use a reverse proxy](#use-a-reverse-proxy): Password-protect a local dashboard and enable TLS to secure it. Full
  support for Netdata Cloud.

## Disable the local dashboard

This is the _recommended method for those who have connected their nodes to Netdata Cloud_ and prefer viewing real-time
metrics using the War Room Overview, Nodes view, and Cloud dashboards.

You can disable the local dashboard (and API) but retain the encrypted Agent-Cloud link ([ACLK](/aclk/README.md)) that
allows you to stream metrics on demand from your nodes via the Netdata Cloud interface. This change mitigates all
concerns about revealing metrics and system design to the internet at large, while keeping all the functionality you
need to view metrics and troubleshoot issues with Netdata Cloud.

Open `netdata.conf` with `./edit-config netdata.conf`. Scroll down to the `[web]` section, and find
the `mode = static-threaded` setting, and change it to `none`.

```conf
[web]
    mode = none
```

Save and close the editor, then [restart your Agent](/docs/configure/start-stop-restart.md)
using `sudo systemctl restart netdata`. If you try to visit the local dashboard to `http://NODE:19999` again, the
connection will fail because that node no longer serves its local dashboard.

> See the [configuration basics doc](/docs/configure/nodes.md) for details on how to find `netdata.conf` and use
> `edit-config`.

## Restrict access to the local dashboard

If you want to keep using the local dashboard, but don't want it exposed to the internet, you can restrict access with
[access lists](/web/server/README.md#access-lists). This method also fully retains the ability to stream metrics
on-demand through Netdata Cloud.

The `allow connections from` setting helps you allow only certain IP addresses or FQDN/hostnames, such as a trusted
static IP, only `localhost`, or connections from behind a management LAN.

By default, this setting is `localhost *`. This setting allows connections from `localhost` in addition to _all_
connections, using the `*` wildcard. You can change this setting using
Netdata's [simple patterns](/libnetdata/simple_pattern/README.md).

```conf
[web]
    # Allow only localhost connections
    allow connections from = localhost

    # Allow only from management LAN running on `10.X.X.X`
    allow connections from = 10.*

    # Allow connections only from a specific FQDN/hostname
    allow connections from = example*
```

## Use your firewall


See the [web server](/web/server/README.md#access-lists) docs for additional details about access lists. You can take
access lists one step further by [enabling SSL](/web/server/README.md#enabling-tls-support) to encrypt data from local
dashboard in transit. The connection to Netdata Cloud is always secured with TLS.

## Use a reverse proxy

You can also put Netdata behind a reverse proxy for additional security while retaining the functionality of both the
local dashboard and Netdata Cloud dashboards. You can use a reverse proxy to password-protect the local dashboard and
enable HTTPS to encrypt metadata and metric values in transit.

We recommend Nginx, as it's what we use for our [demo server](https://london.my-netdata.io/), and we have a guide
dedicated to [running Netdata behind Nginx](/docs/Running-behind-nginx.md).

We also have guides for [Apache](/docs/Running-behind-apache.md), [Lighttpd](/docs/Running-behind-lighttpd.md),
[HAProxy](/docs/Running-behind-haproxy.md), and [Caddy](/docs/Running-behind-caddy.md).

You can put Netdata behind a reverse proxy for additional security while retaining the functionality of both the
local dashboard and Netdata Cloud dashboards. You can use a reverse proxy to password-protect the local dashboard and
enable HTTPS to encrypt metadata and metric values in transit.

We recommend [Nginx](#running-netdata-behind-nginx), as it's what we use for
our [demo server](https://london.my-netdata.io/).
You can also use [Apache](#running-netdata-behind-apache), [Lighttpd](#running-netdata-behind-lighttpd-v14x),
[HAProxy](#running-netdata-behind-haproxy), [H2O](#running-netdata-behind-h2o),
and [Caddy](#running-netdata-behind-caddy).

## Running Netdata behind Nginx

### Nginx configuration file

All Nginx configurations can be found in the `/etc/nginx/` directory. The main configuration file
is `/etc/nginx/nginx.conf`. Website or app-specific configurations can be found in the `/etc/nginx/site-available/`
directory.

Configuration options in Nginx are known as directives. Directives are organized into groups known as blocks or
contexts. The two terms can be used interchangeably.

Depending on your installation source, you’ll find an example configuration file at `/etc/nginx/conf.d/default.conf`
or `etc/nginx/sites-enabled/default`, in some cases you may have to manually create the `sites-available`
and `sites-enabled` directories.

You can edit the Nginx configuration file with Nano, Vim or any other text editors you are comfortable with.

After making changes to the configuration files:

- Test Nginx configuration with `nginx -t`.

- Restart Nginx to effect the change with `/etc/init.d/nginx restart` or `service nginx restart`.

### Ways to access Netdata via Nginx

#### As a virtual host

With this method instead of `SERVER_IP_ADDRESS:19999`, the Netdata dashboard can be accessed via a human-readable URL
such as `netdata.example.com` used in the configuration below.

```conf
upstream backend {
    # the Netdata server
    server 127.0.0.1:19999;
    keepalive 64;
}

server {
    # nginx listens to this
    listen 80;
    # uncomment the line if you want nginx to listen on IPv6 address
    #listen [::]:80;

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

#### As a subfolder to an existing virtual host

This method is recommended when Netdata is to be served from a subfolder (or directory).
In this case, the virtual host `netdata.example.com` already exists and Netdata has to be accessed
via `netdata.example.com/netdata/`.

```conf
upstream netdata {
    server 127.0.0.1:19999;
    keepalive 64;
}

server {
    listen 80;
    # uncomment the line if you want nginx to listen on IPv6 address
    #listen [::]:80;

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

#### As a subfolder for multiple Netdata servers, via one Nginx

This is the recommended configuration when one Nginx will be used to manage multiple Netdata servers via subfolders.

```conf
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
    # uncomment the line if you want nginx to listen on IPv6 address
    #listen [::]:80;

    # the virtual host name of this subfolder should be exposed
    #server_name netdata.example.com;

    location ~ /netdata/(?<behost>.*?)/(?<ndpath>.*) {
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

Of course, you can add as many backend servers as you like.

Using the above, you access Netdata on the backend servers, like this:

- `http://netdata.example.com/netdata/server1/` to reach `backend-server1`
- `http://netdata.example.com/netdata/server2/` to reach `backend-server2`

### Encrypt the communication between Nginx and Netdata

In case Netdata's web server has been [configured to use TLS](/web/server/README.md#enabling-tls-support), it is
necessary to specify inside the Nginx configuration that the final destination is using TLS.

1. To do this, please, append the following parameters in your `nginx.conf`

   ```conf
   proxy_set_header X-Forwarded-Proto https;
   proxy_pass https://localhost:19999;
   ```

2. Optionally it is also possible
   to [enable TLS/SSL on Nginx](http://nginx.org/en/docs/http/configuring_https_servers.html), this way the user will
   encrypt not only the communication between Nginx and Netdata but also between the user and Nginx.

If Nginx is not configured as described here, you will probably receive the error `SSL_ERROR_RX_RECORD_TOO_LONG`.

### Enable authentication

Create an authentication file to enable basic authentication via Nginx, this secures your Netdata dashboard.

If you don't have an authentication file:

1. You can use the following command:

   ```sh
   printf "yourusername:$(openssl passwd -apr1)" > /etc/nginx/passwords
   ```

2. And then enable the authentication inside your server directive:

   ```conf
   server {
       # ...
       auth_basic "Protected";
       auth_basic_user_file passwords;
       # ...
   }
   ```

### Limit direct access to Netdata

If your Nginx is on `localhost`, you can use this to protect your Netdata:

```
[web]
    bind to = 127.0.0.1 ::1
```

You can also use a unix domain socket. This will also provide a faster route between Nginx and Netdata:

```
[web]
    bind to = unix:/var/run/netdata/netdata.sock
```

At the Nginx side, use something like this to use the same unix domain socket:

```conf
upstream backend {
    server unix:/var/run/netdata/netdata.sock;
    keepalive 64;
}
```

If your Nginx server is not on localhost, you can set:

```
[web]
    bind to = *
    allow connections from = IP_OF_NGINX_SERVER
```

`allow connections from`
accepts [Netdata simple patterns](https://github.com/netdata/netdata/blob/master/libnetdata/simple_pattern/README.md) to
match against the connection IP address.

### Prevent the double access.log

Nginx logs accesses and Netdata logs them too. You can prevent Netdata from generating its access log, by setting this
in `/etc/netdata/netdata.conf`:

```
[global]
      access log = none
```

### SELinux

If you get an 502 Bad Gateway error you might check your Nginx error log:

```sh
# cat /var/log/nginx/error.log:
2016/09/09 12:34:05 [crit] 5731#5731: *1 connect() to 127.0.0.1:19999 failed (13: Permission denied) while connecting to upstream, client: 1.2.3.4, server: netdata.example.com, request: "GET / HTTP/2.0", upstream: "http://127.0.0.1:19999/", host: "netdata.example.com"
```

If you see something like the above, chances are high that SELinux prevents nginx from connecting to the backend server.
To fix that, just use this policy: `setsebool -P httpd_can_network_connect true`.

## Running Netdata behind Apache

Below you can find instructions for configuring an apache server to:

1. Proxy a single Netdata via an HTTP and HTTPS virtual host.
2. Dynamically proxy any number of Netdata servers.
3. Add user authentication.
4. Adjust Netdata settings to get optimal results.

### Requirements

Make sure your apache has `mod_proxy` and `mod_proxy_http` installed and enabled.

On Debian/Ubuntu systems, install apache, which already includes the two modules, using:

```sh
sudo apt-get install apache2
```

Enable them:

```sh
sudo a2enmod proxy
sudo a2enmod proxy_http
```

Also, enable the rewrite module:

```sh
sudo a2enmod rewrite
```

### Netdata on an existing virtual host

On any **existing** and already **working** apache virtual host, you can redirect requests for URL `/netdata/` to one or
more Netdata servers.

#### Proxy one Netdata instance, running on the same server apache runs

Add the following on top of any existing virtual host. It will allow you to access Netdata
as `http://virtual.host/netdata/`.

```conf
<VirtualHost *:80>

	RewriteEngine On
	ProxyRequests Off
	ProxyPreserveHost On

	<Proxy *>
		Require all granted
	</Proxy>

	# Local Netdata server accessed with '/netdata/', at localhost:19999
	ProxyPass "/netdata/" "http://localhost:19999/" connectiontimeout=5 timeout=30 keepalive=on
	ProxyPassReverse "/netdata/" "http://localhost:19999/"

	# if the user did not give the trailing /, add it
	# for HTTP (if the virtualhost is HTTP, use this)
	RewriteRule ^/netdata$ http://%{HTTP_HOST}/netdata/ [L,R=301]
	# for HTTPS (if the virtualhost is HTTPS, use this)
	#RewriteRule ^/netdata$ https://%{HTTP_HOST}/netdata/ [L,R=301]

	# rest of virtual host config here
	
</VirtualHost>
```

#### Proxy multiple Netdata instances running on multiple servers

Add the following on top of any existing virtual host. It will allow you to access multiple Netdata
as `http://virtual.host/netdata/HOSTNAME/`, where `HOSTNAME` is the hostname of any other Netdata server you have (to
access the `localhost` Netdata, use `http://virtual.host/netdata/localhost/`).

```conf
<VirtualHost *:80>

	RewriteEngine On
	ProxyRequests Off
	ProxyPreserveHost On

	<Proxy *>
		Require all granted
	</Proxy>

    # proxy any host, on port 19999
    ProxyPassMatch "^/netdata/([A-Za-z0-9\._-]+)/(.*)" "http://$1:19999/$2" connectiontimeout=5 timeout=30 keepalive=on

    # make sure the user did not forget to add a trailing /
    # for HTTP (if the virtualhost is HTTP, use this)
    RewriteRule "^/netdata/([A-Za-z0-9\._-]+)$" http://%{HTTP_HOST}/netdata/$1/ [L,R=301]
    # for HTTPS (if the virtualhost is HTTPS, use this)
    RewriteRule "^/netdata/([A-Za-z0-9\._-]+)$" https://%{HTTP_HOST}/netdata/$1/ [L,R=301]

	# rest of virtual host config here
	
</VirtualHost>
```

> IMPORTANT<br/>
> The above config allows your apache users to connect to port 19999 on any server on your network.

If you want to control the servers your users can connect to, replace the `ProxyPassMatch` line with the following. This
allows only `server1`, `server2`, `server3` and `server4`.

```
    ProxyPassMatch "^/netdata/(server1|server2|server3|server4)/(.*)" "http://$1:19999/$2" connectiontimeout=5 timeout=30 keepalive=on
```

### Netdata on a dedicated virtual host

You can proxy Netdata through apache, using a dedicated apache virtual host.

1. Create a new apache site:

   ```sh
   nano /etc/apache2/sites-available/netdata.conf
   ```

   with this content:

   ```conf
   <VirtualHost *:80>
       ProxyRequests Off
       ProxyPreserveHost On
       
       ServerName netdata.domain.tld
   
       <Proxy *>
           Require all granted
       </Proxy>
   
       ProxyPass "/" "http://localhost:19999/" connectiontimeout=5 timeout=30 keepalive=on
       ProxyPassReverse "/" "http://localhost:19999/"
   
       ErrorLog ${APACHE_LOG_DIR}/netdata-error.log
       CustomLog ${APACHE_LOG_DIR}/netdata-access.log combined
   </VirtualHost>
   ```

2. Enable the VirtualHost:

   ```sh
   sudo a2ensite netdata.conf && service apache2 reload
   ```

### Netdata proxy in Plesk

_Assuming the main goal is to make Netdata running in HTTPS._

1. Make a subdomain for Netdata on which you enable and force HTTPS - You can use a free Let's Encrypt certificate
2. Go to "Apache & nginx Settings", and in the following section, add:

    ```conf
    RewriteEngine on
    RewriteRule (.*) http://localhost:19999/$1 [P,L]
    ```

3. Optional: If your server is remote, then just replace "localhost" with your actual hostname or IP, it just works.

Repeat the operation for as many servers as you need.

### Enable Basic Auth

If you wish to add an authentication (user/password) to access your Netdata, do these:

1. Install the package `apache2-utils`. On Debian/Ubuntu run `sudo apt-get install apache2-utils`.

2. Then, generate password for user `netdata`, using `htpasswd -c /etc/apache2/.htpasswd netdata`

<details><summary>Apache 2.2 Example:</summary>

Modify the virtual host with:

```conf
	# replace the <Proxy *> section
	<Proxy *>
		Order deny,allow
		Allow from all
	</Proxy>

	# add a <Location /netdata/> section
	<Location /netdata/>
		AuthType Basic
		AuthName "Protected site"
		AuthUserFile /etc/apache2/.htpasswd
		Require valid-user
		Order deny,allow
		Allow from all
	</Location>
```

Specify `Location /` if Netdata is running on dedicated virtual host.

</details>

<details><summary>Apache 2.4 (dedicated virtual host) Example:</summary>

```conf
<VirtualHost *:80>
	RewriteEngine On
	ProxyRequests Off
	ProxyPreserveHost On
	
	ServerName netdata.domain.tld

	<Proxy *>
		AllowOverride None
		AuthType Basic
		AuthName "Protected site"
		AuthUserFile /etc/apache2/.htpasswd
		Require valid-user
	</Proxy>

	ProxyPass "/" "http://localhost:19999/" connectiontimeout=5 timeout=30 keepalive=on
	ProxyPassReverse "/" "http://localhost:19999/"

	ErrorLog ${APACHE_LOG_DIR}/netdata-error.log
	CustomLog ${APACHE_LOG_DIR}/netdata-access.log combined
</VirtualHost>
```

</details>

Note: Changes are applied by reloading or restarting Apache.

### Configuration of Content Security Policy

If you want to enable CSP within your Apache, you should consider some special requirements of the headers. Modify your
configuration like that:

```
	Header always set Content-Security-Policy "default-src http: 'unsafe-inline' 'self' 'unsafe-eval'; script-src http: 'unsafe-inline' 'self' 'unsafe-eval'; style-src http: 'self' 'unsafe-inline'"
```

Note: Changes are applied by reloading or restarting Apache.

### Using Netdata with Apache's `mod_evasive` module

The `mod_evasive` Apache module helps system administrators protect their web server from brute force and distributed
denial of service attack (DDoS) attacks.

Because Netdata sends a request to the web server for every chart update, it's normal to create 20-30 requests per
second, per client. If you're using `mod_evasive` on your Apache web server, this volume of requests will trigger the
module's protection, and your dashboard will become unresponsive. You may even begin to see 403 errors.

To mitigate this issue, you will need to change the value of the `DOSPageCount` option in your `mod_evasive.conf` file,
which can typically be found at `/etc/httpd/conf.d/mod_evasive.conf` or `/etc/apache2/mods-enabled/evasive.conf`.

The `DOSPageCount` option sets the limit of the number of requests from a single IP address for the same page per page
interval, which is usually 1 second. The default value is `2` requests per second. Clearly, Netdata's typical usage will
exceed that threshold, and `mod_evasive` will add your IP address to a blocklist.

Our users have found success by setting `DOSPageCount` to `30`. Try this, and raise the value if you continue to see 403
errors while accessing the dashboard.

```conf
DOSPageCount 30
```

Restart Apache with `sudo systemctl restart apache2`, or the appropriate method to restart services on your system, to
reload its configuration with your new values.

#### Virtual host

To adjust the `DOSPageCount` for a specific virtual host, open your virtual host config, which can be found at
`/etc/httpd/conf/sites-available/my-domain.conf` or `/etc/apache2/sites-available/my-domain.conf` and add the
following:

```conf
<VirtualHost *:80>
	...
	# Increase the DOSPageCount to prevent 403 errors and IP addresses being blocked.
	<IfModule mod_evasive20.c>
		DOSPageCount        30
	</IfModule>
</VirtualHost>
```

See issues [#2011](https://github.com/netdata/netdata/issues/2011) and
[#7658](https://github.com/netdata/netdata/issues/7568) for more information.

### Netdata configuration

You might edit `/etc/netdata/netdata.conf` to optimize your setup a bit. For applying these changes you need to restart
Netdata.

#### Response compression

If you plan to use Netdata exclusively via apache, you can gain some performance by preventing double compression of its
output (Netdata compresses its response, apache re-compresses it) by editing `/etc/netdata/netdata.conf` and setting:

```
[web]
    enable gzip compression = no
```

Once you disable compression at Netdata (and restart it), please verify you receive compressed responses from apache (it
is important to receive compressed responses - the charts will be more snappy).

#### Limit direct access to Netdata

You would also need to instruct Netdata to listen only on `localhost`, `127.0.0.1` or `::1`.

```
[web]
    bind to = localhost
```

or

```
[web]
    bind to = 127.0.0.1
```

or

```
[web]
    bind to = ::1
```

You can also use a unix domain socket. This will also provide a faster route between apache and Netdata:

```
[web]
    bind to = unix:/tmp/netdata.sock
```

Apache 2.4.24+ can not read from `/tmp` so create your socket in `/var/run/netdata`

```
[web]
    bind to = unix:/var/run/netdata/netdata.sock
```

_note: Netdata v1.8+ support unix domain sockets_

At the apache side, prepend the 2nd argument to `ProxyPass` with `unix:/tmp/netdata.sock|`, like this:

```
ProxyPass "/netdata/" "unix:/tmp/netdata.sock|http://localhost:19999/" connectiontimeout=5 timeout=30 keepalive=on
```

If your apache server is not on localhost, you can set:

```
[web]
    bind to = *
    allow connections from = IP_OF_APACHE_SERVER
```

*note: Netdata v1.9+ support `allow connections from`*

`allow connections from` accepts [Netdata simple patterns](/libnetdata/simple_pattern/README.md) to match against the
connection IP address.

#### prevent the double access.log

apache logs accesses and Netdata logs them too. You can prevent Netdata from generating its access log, by setting this
in `/etc/netdata/netdata.conf`:

```
[global]
    access log = none
```

#### Troubleshooting mod_proxy

Make sure the requests reach Netdata, by examining `/var/log/netdata/access.log`.

1. if the requests do not reach Netdata, your apache does not forward them.
2. if the requests reach Netdata but the URLs are wrong, you have not re-written them properly.

## Running Netdata behind lighttpd v1.4.x

Here is a config for accessing Netdata in a suburl via lighttpd 1.4.46 and newer:

```txt
$HTTP["url"] =~ "^/netdata/" {
    proxy.server  = ( "" => ("netdata" => ( "host" => "127.0.0.1", "port" => 19999 )))
    proxy.header = ( "map-urlpath" => ( "/netdata/" => "/") )
}
```

If you have older lighttpd you have to use a chain (such as below), as
explained [at this stackoverflow answer](http://stackoverflow.com/questions/14536554/lighttpd-configuration-to-proxy-rewrite-from-one-domain-to-another)
.

```txt
$HTTP["url"] =~ "^/netdata/" {
    proxy.server  = ( "" => ("" => ( "host" => "127.0.0.1", "port" => 19998 )))
}

$SERVER["socket"] == ":19998" {
    url.rewrite-once = ( "^/netdata(.*)$" => "/$1" )
    proxy.server = ( "" => ( "" => ( "host" => "127.0.0.1", "port" => 19999 )))
}
```

If the only thing the server is exposing via the web is Netdata (and thus no suburl rewriting required),
then you can get away with just:

```
proxy.server  = ( "" => ( ( "host" => "127.0.0.1", "port" => 19999 )))
```

Though if it's public facing you might then want to put some authentication on it. htdigest support
looks like:

```
auth.backend = "htdigest"
auth.backend.htdigest.userfile = "/etc/lighttpd/lighttpd.htdigest"
auth.require = ( "" => ( "method" => "digest", 
                         "realm" => "netdata", 
                         "require" => "valid-user" 
                       )
               )
```

other auth methods, and more info on htdigest, can be found in
lighttpd's [mod_auth docs](http://redmine.lighttpd.net/projects/lighttpd/wiki/Docs_ModAuth).

It seems that lighttpd (or some versions of it), fail to proxy compressed web responses.
To solve this issue, disable web response compression in Netdata.

Open `/etc/netdata/netdata.conf` and set in `[global]\`:

```
enable web responses gzip compression = no
```

### limit direct access to Netdata

You would also need to instruct Netdata to listen only to `127.0.0.1` or `::1`.

To limit access to Netdata only from localhost, set `bind socket to IP = 127.0.0.1` or `bind socket to IP = ::1`
in `/etc/netdata/netdata.conf`.

## Running Netdata behind HAProxy

> HAProxy is a free, very fast and reliable solution offering high availability, load balancing,
> and proxying for TCP and HTTP-based applications. It is particularly suited for very high traffic websites
> and powers quite a number of the world's most visited ones.

If Netdata is running on a host running HAProxy, rather than connecting to Netdata from a port number, a domain name can
be pointed at HAProxy, and HAProxy can redirect connections to the Netdata port. This can make it possible to connect to
Netdata at `https://example.com` or `https://example.com/netdata/`, which is a much nicer experience than
`http://example.com:19999`.

To proxy requests from [HAProxy](https://github.com/haproxy/haproxy) to Netdata,
the following configuration can be used:

### Default Configuration

For all examples, set the mode to `http`

```conf
defaults
    mode    http
```

### Simple Configuration

A simple example where the base URL, say `http://example.com`, is used with no subpath:

#### Frontend

Create a frontend to receive the request.

```conf
frontend http_frontend
    ## HTTP ipv4 and ipv6 on all ips ##
    bind :::80 v4v6

    default_backend     netdata_backend
```

#### Backend

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

### Configuration with subpath

An example where the base URL is used with a subpath `/netdata/`:

#### Frontend

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

#### Backend

Same as simple example, except remove `/netdata/` with regex.

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

### Using TLS communication

TLS can be used by adding port `443` and a cert to the frontend.
This example will only use Netdata if host matches example.com (replace with your domain).

#### Frontend

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

#### Backend

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

### Enable authentication

To use basic HTTP Authentication, create an authentication list:

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

### Full Example

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

## Running Netdata behind Caddy

To run Netdata via [Caddy v2 proxying,](https://caddyserver.com/docs/caddyfile/directives/reverse_proxy) set your
Caddyfile up like this:

```caddyfile
netdata.domain.tld {
    reverse_proxy localhost:19999
}
```

Other directives can be added between the curly brackets as needed.

To run Netdata in a subfolder:

```caddyfile
netdata.domain.tld {
    handle_path /netdata/* {
        reverse_proxy localhost:19999
    }
}
```

### limit direct access to Netdata

You would also need to instruct Netdata to listen only to `127.0.0.1` or `::1`.

To limit access to Netdata only from localhost, set `bind socket to IP = 127.0.0.1` or `bind socket to IP = ::1`
in `/etc/netdata/netdata.conf`.

## Running Netdata behind H2O

[H2O](https://h2o.examp1e.net/) is a new generation HTTP server that provides quicker response to users with less CPU
utilization when compared to older generation of web servers.

It is notable for having much simpler configuration than many popular HTTP servers, low resource requirements, and
integrated native support for many things that other HTTP servers may need special setup to use.

### H2O configuration file.

On most systems, the H2O configuration is found under `/etc/h2o`. H2O uses [YAML 1.1](https://yaml.org/spec/1.1/), with
a few special extensions, for it’s configuration files, with the main configuration file being `/etc/h2o/h2o.conf`.

You can edit the H2O configuration file with Nano, Vim or any other text editors with which you are comfortable.

After making changes to the configuration files, perform the following:

- Test the configuration with `h2o -m test -c /etc/h2o/h2o.conf`

- Restart H2O to apply tha changes with `/etc/init.d/h2o restart` or `service h2o restart`

### Ways to access Netdata via H2O

#### As a virtual host

With this method instead of `SERVER_IP_ADDRESS:19999`, the Netdata dashboard can be accessed via a human-readable URL
such as `netdata.example.com` used in the configuration below.

```yaml
hosts:
  netdata.example.com:
    listen:
      port: 80
    paths:
      /:
        proxy.preserve-host: ON
        proxy.reverse.url: http://127.0.0.1:19999
```

#### As a subfolder of an existing virtual host

This method is recommended when Netdata is to be served from a subfolder (or directory).
In this case, the virtual host `netdata.example.com` already exists and Netdata has to be accessed
via `netdata.example.com/netdata/`.

```yaml
hosts:
  netdata.example.com:
    listen:
      port: 80
    paths:
      /netdata:
        redirect:
          status: 301
          url: /netdata/
      /netdata/:
        proxy.preserve-host: ON
        proxy.reverse.url: http://127.0.0.1:19999
```

#### As a subfolder for multiple Netdata servers, via one H2O instance

This is the recommended configuration when one H2O instance will be used to manage multiple Netdata servers via
subfolders.

```yaml
hosts:
  netdata.example.com:
    listen:
      port: 80
    paths:
      /netdata/server1:
        redirect:
          status: 301
          url: /netdata/server1/
      /netdata/server1/:
        proxy.preserve-host: ON
        proxy.reverse.url: http://198.51.100.1:19999
      /netdata/server2:
        redirect:
          status: 301
          url: /netdata/server2/
      /netdata/server2/:
        proxy.preserve-host: ON
        proxy.reverse.url: http://198.51.100.2:19999
```

Of course, you can add as many backend servers as you like.

Using the above, you access Netdata on the backend servers, like this:

- `http://netdata.example.com/netdata/server1/` to reach Netdata on `198.51.100.1:19999`
- `http://netdata.example.com/netdata/server2/` to reach Netdata on `198.51.100.2:19999`

### Encrypt the communication between H2O and Netdata

In case Netdata's web server has been [configured to use TLS](/web/server/README.md#enabling-tls-support), it is
necessary to specify inside the H2O configuration that the final destination is using TLS. To do this, change the
`http://` on the `proxy.reverse.url` line in your H2O configuration with `https://`

### Enable authentication

Create an authentication file to enable basic authentication via H2O, this secures your Netdata dashboard.

If you don't have an authentication file, you can use the following command:

```sh
printf "yourusername:$(openssl passwd -apr1)" > /etc/h2o/passwords
```

And then add a basic authentication handler to each path definition:

```yaml
hosts:
  netdata.example.com:
    listen:
      port: 80
    paths:
      /:
        mruby.handler: |
          require "htpasswd.rb"
          Htpasswd.new("/etc/h2o/passwords", "netdata.example.com")
        proxy.preserve-host: ON
        proxy.reverse.url: http://127.0.0.1:19999
```

For more information on using basic authentication with H2O,
see [their official documentation](https://h2o.examp1e.net/configure/basic_auth.html).

### Limit direct access to Netdata

If your H2O server is on `localhost`, you can use this to ensure external access is only possible through H2O:

```
[web]
    bind to = 127.0.0.1 ::1
```

You can also use a unix domain socket. This will provide faster communication between H2O and Netdata as well:

```
[web]
    bind to = unix:/run/netdata/netdata.sock
```

In the H2O configuration, use a line like the following to connect to Netdata via the unix socket:

```yaml
proxy.reverse.url http://[unix:/run/netdata/netdata.sock]
```

If your H2O server is not on localhost, you can set:

```
[web]
    bind to = *
    allow connections from = IP_OF_H2O_SERVER
```

*note: Netdata v1.9+ support `allow connections from`*

`allow connections from` accepts [Netdata simple patterns](/libnetdata/simple_pattern/README.md) to match against
the connection IP address.

### Prevent the double access.log

H2O logs accesses and Netdata logs them too. You can prevent Netdata from generating its access log, by setting
this in `/etc/netdata/netdata.conf`:

```
[global]
      access log = none
```