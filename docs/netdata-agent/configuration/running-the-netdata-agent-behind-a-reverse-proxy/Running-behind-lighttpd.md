# Running Netdata behind lighttpd v1.4.x

Here is a config for accessing Netdata in a suburl via lighttpd 1.4.46 and newer:

```text
$HTTP["url"] =~ "^/netdata/" {
    proxy.server  = ( "" => ("netdata" => ( "host" => "127.0.0.1", "port" => 19999 )))
    proxy.header = ( "map-urlpath" => ( "/netdata/" => "/") )
}
```

If you have older lighttpd, you have to use a chain (such as below), as explained [at this Stack Overflow answer](http://stackoverflow.com/questions/14536554/lighttpd-configuration-to-proxy-rewrite-from-one-domain-to-another).

```text
$HTTP["url"] =~ "^/netdata/" {
    proxy.server  = ( "" => ("" => ( "host" => "127.0.0.1", "port" => 19998 )))
}

$SERVER["socket"] == ":19998" {
    url.rewrite-once = ( "^/netdata(.*)$" => "/$1" )
    proxy.server = ( "" => ( "" => ( "host" => "127.0.0.1", "port" => 19999 )))
}
```

If the only thing the server is exposing via the web is Netdata (and thus no suburl rewriting required),
then you can get away with just

```text
proxy.server  = ( "" => ( ( "host" => "127.0.0.1", "port" => 19999 )))
```

Though if it's public facing, you might then want to put some authentication on it. `htdigest` support looks like:

```text
auth.backend = "htdigest"
auth.backend.htdigest.userfile = "/etc/lighttpd/lighttpd.htdigest"
auth.require = ( "" => ( "method" => "digest", 
                         "realm" => "netdata", 
                         "require" => "valid-user" 
                       )
               )
```

other auth methods, and more info on htdigest, can be found in lighttpd's [mod_auth docs](http://redmine.lighttpd.net/projects/lighttpd/wiki/Docs_ModAuth).

It seems that lighttpd (or some versions of it), fail to proxy compressed web responses.
To solve this issue, disable web response compression in Netdata.

Open `/etc/netdata/netdata.conf` and set in `[global]`:

```text
enable web responses gzip compression = no
```

## limit direct access to Netdata

You would also need to instruct Netdata to listen only to `127.0.0.1` or `::1`.

To limit access to Netdata only from localhost, set `bind socket to IP = 127.0.0.1` or `bind socket to IP = ::1` in `/etc/netdata/netdata.conf`.
