# Netdata via apache's mod_proxy

Below you can find instructions for configuring an apache server to:

1. proxy a single netdata via an HTTP and HTTPS virtual host
2. dynamically proxy any number of netdata
3. add user authentication
4. adjust netdata settings to get optimal results


## Requirements

Make sure your apache has installed `mod_proxy` and `mod_proxy_http`.

On debian/ubuntu systems, install them with this: 

```sh
sudo apt-get install libapache2-mod-proxy-html
```

Also make sure they are enabled:

```
sudo a2enmod proxy
sudo a2enmod proxy_http
```

Ensure your rewrite module is enabled:

```
sudo a2enmod rewrite
```

---

## netdata on an existing virtual host

On any **existing** and already **working** apache virtual host, you can redirect requests for URL `/netdata/` to one or more netdata servers.

### proxy one netdata, running on the same server apache runs

Add the following on top of any existing virtual host. It will allow you to access netdata as `http://virtual.host/netdata/`.

```
<VirtualHost *:80>

	RewriteEngine On
	ProxyRequests Off
	ProxyPreserveHost On

	<Proxy *>
		Require all granted
	</Proxy>

	# Local netdata server accessed with '/netdata/', at localhost:19999
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

### proxy multiple netdata running on multiple servers

Add the following on top of any existing virtual host. It will allow you to access multiple netdata as `http://virtual.host/netdata/HOSTNAME/`, where `HOSTNAME` is the hostname of any other netdata server you have (to access the `localhost` netdata, use `http://virtual.host/netdata/localhost/`).

```
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

## netdata on a dedicated virtual host

You can proxy netdata through apache, using a dedicated apache virtual host.

Create a new apache site:

```sh
nano /etc/apache2/sites-available/netdata.conf
```

with this content:

```
<VirtualHost *:80>
	RewriteEngine On
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
1. Make a subdomain for Netdata on which you enable and force HTTPS - You can use a free Let's Encrypt certificate
2. Go to "Apache & nginx Settings", and in the following section, add:
```
RewriteEngine on
RewriteRule (.*) http://localhost:19999/$1 [P,L]
```
3. Optional: If your server is remote, then just replace "localhost" with your actual hostname or IP, it just works.

Repeat the operation for as many servers as you need.


## Enable Basic Auth

If you wish to add an authentication (user/password) to access your netdata, do these:

Install the package `apache2-utils`. On debian / ubuntu run `sudo apt-get install apache2-utils`.

Then, generate password for user `netdata`, using `htpasswd -c /etc/apache2/.htpasswd netdata`

Modify the virtual host with these:

```
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

Specify `Location /` if netdata is running on dedicated virtual host.

Note: Changes are applied by reloading or restarting Apache.

# Netdata configuration

You might edit `/etc/netdata/netdata.conf` to optimize your setup a bit. For applying these changes you need to restart netdata.

## Response compression

If you plan to use netdata exclusively via apache, you can gain some performance by preventing double compression of its output (netdata compresses its response, apache re-compresses it) by editing `/etc/netdata/netdata.conf` and setting:

```
[web]
    enable gzip compression = no
```

Once you disable compression at netdata (and restart it), please verify you receive compressed responses from apache (it is important to receive compressed responses - the charts will be more snappy).

## Limit direct access to netdata

You would also need to instruct netdata to listen only on `localhost`, `127.0.0.1` or `::1`.

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

---

You can also use a unix domain socket. This will also provide a faster route between apache and netdata:

```
[web]
    bind to = unix:/tmp/netdata.sock
```
_note: netdata v1.8+ support unix domain sockets_

At the apache side, prepend the 2nd argument to `ProxyPass` with `unix:/tmp/netdata.sock|`, like this:

```
ProxyPass "/netdata/" "unix:/tmp/netdata.sock|http://localhost:19999/" connectiontimeout=5 timeout=30 keepalive=on
```

---

If your apache server is not on localhost, you can set:

```
[web]
    bind to = *
    allow connections from = IP_OF_APACHE_SERVER
```
_note: netdata v1.9+ support `allow connections from`_

`allow connections from` accepts [netdata simple patterns](../libnetdata/simple_pattern/) to match against the connection IP address.

## prevent the double access.log

apache logs accesses and netdata logs them too. You can prevent netdata from generating its access log, by setting this in `/etc/netdata/netdata.conf`:

```
[global]
    access log = none
```

## Troubleshooting mod_proxy

Make sure the requests reach netdata, by examing `/var/log/netdata/access.log`.

1. if the requests do not reach netdata, your apache does not forward them.
2. if the requests reach netdata by the URLs are wrong, you have not re-written them properly.

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fdocs%2FRunning-behind-apache&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)]()
