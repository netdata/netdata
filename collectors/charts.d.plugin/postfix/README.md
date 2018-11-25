# postfix

> THIS MODULE IS OBSOLETE.
> USE [THE PYTHON ONE](../../python.d.plugin/postfix) - IT SUPPORTS MULTIPLE JOBS AND IT IS MORE EFFICIENT

The plugin will collect the postfix queue size.

It will create two charts:

1. **queue size in emails**
2. **queue size in KB**

### configuration

This is the internal default for `/etc/netdata/postfix.conf`

```sh
# the postqueue command
# if empty, it will use the one found in the system path
postfix_postqueue=

# how frequently to collect queue size
postfix_update_every=15
```

---
