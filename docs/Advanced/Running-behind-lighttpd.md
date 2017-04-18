# lighttpd v1.4.x

Here is a config for accessing netdata via lighttpd 1.4.x:

```txt
$HTTP["url"] =~ "^/netdata/" {
    proxy.server  = ( "" => ("" => ( "host" => "127.0.0.1", "port" => 19998 )))
}

$SERVER["socket"] == ":19998" {
    url.rewrite-once = ( "^/netdata(.*)$" => "/$1" )
    proxy.server = ( "" => ( "" => ( "host" => "127.0.0.1", "port" => 19999 )))
}
```

As you see you have to use a chain, as explained [at this stackoverflow answer](http://stackoverflow.com/questions/14536554/lighttpd-configuration-to-proxy-rewrite-from-one-domain-to-another).

---

It seems that lighttpd (or some versions of it), fail to proxy compressed web responses.
To solve this issue, disable web response compression in netdata.

Open /etc/netdata/netdata.conf and set in [global]:

```
enable web responses gzip compression = no
```

## limit direct access to netdata

You would also need to instruct netdata to listen only to `127.0.0.1` or `::1`.

To limit access to netdata only from localhost, set `bind socket to IP = 127.0.0.1` or `bind socket to IP = ::1` in `/etc/netdata/netdata.conf`.
