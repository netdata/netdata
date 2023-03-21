# Running Netdata behind a reverse proxy

If you need to access a Netdata agent's user interface or API in a production environment we recommend you put Netdata behind
another web server and secure access to the dashboard via SSL and user authentication. A dedicated web server also provides more robustness 
and capabilities than the Agent's internal [web server](https://github.com/netdata/netdata/blob/master/web/README.md).

We have documented running behind
[nginx](https://github.com/netdata/netdata/blob/master/docs/Running-behind-nginx.md),
[Apache](https://github.com/netdata/netdata/blob/master/docs/Running-behind-apache.md),
[HAProxy](https://github.com/netdata/netdata/blob/master/docs/Running-behind-haproxy.md),
[Lighttpd](https://github.com/netdata/netdata/blob/master/docs/Running-behind-lighttpd.md),
[Caddy](https://github.com/netdata/netdata/blob/master/docs/Running-behind-caddy.md),
and [H2O](https://github.com/netdata/netdata/blob/master/docs/Running-behind-h2o.md).
If you prefer a different web server, we suggest you follow the documentation for nginx and tell us how you did it 
 by adding your own "Running behind webserverX" document.
 
