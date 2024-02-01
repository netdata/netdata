# Netdata via Apache's mod_proxy

Below you can find instructions for configuring an apache server to:

1. Proxy a single Netdata via an HTTP and HTTPS virtual host.
2. Dynamically proxy any number of Netdata servers.
3. Add user authentication.
4. Adjust Netdata settings to get optimal results.

## Requirements

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
## Netdata on an existing virtual host

On any **existing** and already **working** apache virtual host, you can redirect requests for URL `/netdata/` to one or more Netdata servers.

### Proxy one Netdata, running on the same server apache runs

Add the following on top of any existing virtual host. It will allow you to access Netdata as `http://virtual.host/netdata/`.

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

### Proxy multiple Netdata running on multiple servers

Add the following on top of any existing virtual host. It will allow you to access multiple Netdata as `http://virtual.host/netdata/HOSTNAME/`, where `HOSTNAME` is the hostname of any other Netdata server you have (to access the `localhost` Netdata, use `http://virtual.host/netdata/localhost/`).

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

If you want to control the servers your users can connect to, replace the `ProxyPassMatch` line with the following. This allows only `server1`, `server2`, `server3` and `server4`.

```
    ProxyPassMatch "^/netdata/(server1|server2|server3|server4)/(.*)" "http://$1:19999/$2" connectiontimeout=5 timeout=30 keepalive=on
```

## Netdata on a dedicated virtual host

You can proxy Netdata through apache, using a dedicated apache virtual host.

Create a new apache site:

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

Enable the VirtualHost: 

```sh
sudo a2ensite netdata.conf && service apache2 reload
```

## Netdata proxy in Plesk

_Assuming the main goal is to make Netdata running in HTTPS._

1.  Make a subdomain for Netdata on which you enable and force HTTPS - You can use a free Let's Encrypt certificate
2.  Go to "Apache & nginx Settings", and in the following section, add:

```conf
RewriteEngine on
RewriteRule (.*) http://localhost:19999/$1 [P,L]
```

3.  Optional: If your server is remote, then just replace "localhost" with your actual hostname or IP, it just works.  

Repeat the operation for as many servers as you need.

## Enable Basic Auth

If you wish to add an authentication (user/password) to access your Netdata, do these:

Install the package `apache2-utils`. On Debian/Ubuntu run `sudo apt-get install apache2-utils`.

Then, generate password for user `netdata`, using `htpasswd -c /etc/apache2/.htpasswd netdata`

**Apache 2.2 Example:**\
Modify the virtual host with these:

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

**Apache 2.4 (dedicated virtual host) Example:**  

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

Note: Changes are applied by reloading or restarting Apache.

## Configuration of Content Security Policy

If you want to enable CSP within your Apache, you should consider some special requirements of the headers. Modify your configuration like that:

```
	Header always set Content-Security-Policy "default-src http: 'unsafe-inline' 'self' 'unsafe-eval'; script-src http: 'unsafe-inline' 'self' 'unsafe-eval'; style-src http: 'self' 'unsafe-inline'"
```

Note: Changes are applied by reloading or restarting Apache.

## Using Netdata with Apache's `mod_evasive` module

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

### Virtual host

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

# Netdata configuration

You might edit `/etc/netdata/netdata.conf` to optimize your setup a bit. For applying these changes you need to restart Netdata.

## Response compression

If you plan to use Netdata exclusively via apache, you can gain some performance by preventing double compression of its output (Netdata compresses its response, apache re-compresses it) by editing `/etc/netdata/netdata.conf` and setting:

```
[web]
    enable gzip compression = no
```

Once you disable compression at Netdata (and restart it), please verify you receive compressed responses from apache (it is important to receive compressed responses - the charts will be more snappy).

## Limit direct access to Netdata

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

`allow connections from` accepts [Netdata simple patterns](https://github.com/netdata/netdata/blob/master/src/libnetdata/simple_pattern/README.md) to match against the connection IP address.

## Prevent the double access.log

apache logs accesses and Netdata logs them too. You can prevent Netdata from generating its access log, by setting this in `/etc/netdata/netdata.conf`:

```
[global]
    access log = none
```

## Troubleshooting mod_proxy

Make sure the requests reach Netdata, by examining `/var/log/netdata/access.log`.

1.  if the requests do not reach Netdata, your apache does not forward them.
2.  if the requests reach Netdata but the URLs are wrong, you have not re-written them properly.


