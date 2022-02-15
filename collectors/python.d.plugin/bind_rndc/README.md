<!--
title: "ISC Bind monitoring with Netdata"
custom_edit_url: https://github.com/netdata/netdata/edit/master/collectors/python.d.plugin/bind_rndc/README.md
sidebar_label: "ISC Bind"
-->

# ISC Bind monitoring with Netdata

Collects Name server summary performance statistics using `rndc` tool.

## Requirements

-   Version of bind must be 9.6 +
-   Netdata must have permissions to run `rndc stats`

It produces:

1.  **Name server statistics**

    -   requests
    -   responses
    -   success
    -   auth_answer
    -   nonauth_answer
    -   nxrrset
    -   failure
    -   nxdomain
    -   recursion
    -   duplicate
    -   rejections

2.  **Incoming queries**

    -   RESERVED0
    -   A
    -   NS
    -   CNAME
    -   SOA
    -   PTR
    -   MX
    -   TXT
    -   X25
    -   AAAA
    -   SRV
    -   NAPTR
    -   A6
    -   DS
    -   RSIG
    -   DNSKEY
    -   SPF
    -   ANY
    -   DLV

3.  **Outgoing queries**

-   Same as Incoming queries

## Configuration

Edit the `python.d/bind_rndc.conf` configuration file using `edit-config` from the Netdata [config
directory](/docs/configure/nodes.md), which is typically at `/etc/netdata`.

```bash
cd /etc/netdata   # Replace this path with your Netdata config directory, if different
sudo ./edit-config python.d/bind_rndc.conf
```

Sample:

```yaml
local:
  named_stats_path       : '/var/log/bind/named.stats'
```

If no configuration is given, module will attempt to read named.stats file  at `/var/log/bind/named.stats`

---


