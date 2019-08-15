# phpfpm

This module will monitor one or more php-fpm instances depending on configuration.

**Requirements:**

-   php-fpm with enabled `status` page
-   access to `status` page via web server

It produces following charts:

1.  **Active Connections**

    -   active
    -   maxActive
    -   idle

2.  **Requests** in requests/s

    -   requests

3.  **Performance**

    -   reached
    -   slow

## configuration

Needs only `url` to server's `status`

Here is an example for local instance:

```yaml
update_every : 3
priority     : 90100

local:
  url     : 'http://localhost/status'
```

Without configuration, module attempts to connect to `http://localhost/status`

---

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fcollectors%2Fpython.d.plugin%2Fphpfpm%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
