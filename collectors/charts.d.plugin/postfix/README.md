> THIS MODULE IS OBSOLETE.
> USE THE PYTHON ONE - IT SUPPORTS MULTIPLE JOBS AND IT IS MORE EFFICIENT

# postfix

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
