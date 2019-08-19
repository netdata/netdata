# postfix

> THIS MODULE IS OBSOLETE.
> USE [THE PYTHON ONE](../../python.d.plugin/postfix) - IT SUPPORTS MULTIPLE JOBS AND IT IS MORE EFFICIENT

The plugin will collect the postfix queue size.

It will create two charts:

1.  **queue size in emails**
2.  **queue size in KB**

## configuration

This is the internal default for `/etc/netdata/postfix.conf`

```sh
# the postqueue command
# if empty, it will use the one found in the system path
postfix_postqueue=

# how frequently to collect queue size
postfix_update_every=15
```

---

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fcollectors%2Fcharts.d.plugin%2Fpostfix%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
