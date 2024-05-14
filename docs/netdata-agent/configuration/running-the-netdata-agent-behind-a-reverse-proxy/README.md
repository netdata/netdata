# Running the Netdata Agent behind a reverse proxy

If you need to access a Netdata agent's user interface or API in a production environment we recommend you put Netdata behind
another web server and secure access to the dashboard via SSL, user authentication and firewall rules. 

A dedicated web server also provides more robustness and capabilities than the Agent's [internal web server](https://github.com/netdata/netdata/blob/master/src/web/README.md).

We have documented running behind
[nginx](https://github.com/netdata/netdata/blob/master/docs/Running-behind-nginx.md),
[Apache](https://github.com/netdata/netdata/blob/master/docs/Running-behind-apache.md),
[HAProxy](https://github.com/netdata/netdata/blob/master/docs/Running-behind-haproxy.md),
[Lighttpd](https://github.com/netdata/netdata/blob/master/docs/Running-behind-lighttpd.md),
[Caddy](https://github.com/netdata/netdata/blob/master/docs/Running-behind-caddy.md),
and [H2O](https://github.com/netdata/netdata/blob/master/docs/Running-behind-h2o.md).
If you prefer a different web server, we suggest you follow the documentation for nginx and tell us how you did it 
 by adding your own "Running behind webserverX" document.

When you run Netdata behind a reverse proxy, we recommend you firewall protect all your Netdata servers, so that only the web server IP will be allowed to directly access Netdata. To do this, run this on each of your servers (or use your firewall manager):

```sh
PROXY_IP="1.2.3.4"
iptables -t filter -I INPUT -p tcp --dport 19999 \! -s ${PROXY_IP} -m conntrack --ctstate NEW -j DROP
```

The above will prevent anyone except your web server to access a Netdata dashboard running on the host.

You can also use `netdata.conf`:

```
[web]
	allow connections from = localhost 1.2.3.4
```

Of course, you can add more IPs.
