<!--
title: "ISC Bind monitoring with Netdata"
custom_edit_url: "https://github.com/netdata/netdata/edit/master/collectors/python.d.plugin/bind_rndc/README.md"
sidebar_label: "ISC Bind"
learn_status: "Published"
learn_topic_type: "References"
learn_rel_path: "Integrations/Monitor/Webapps"
-->

# ISC Bind collector

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
directory](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md), which is typically at `/etc/netdata`.

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




### Troubleshooting

To troubleshoot issues with the `bind_rndc` module, run the `python.d.plugin` with the debug option enabled. The 
output will give you the output of the data collection job or error messages on why the collector isn't working.

First, navigate to your plugins directory, usually they are located under `/usr/libexec/netdata/plugins.d/`. If that's 
not the case on your system, open `netdata.conf` and look for the setting `plugins directory`. Once you're in the 
plugin's directory, switch to the `netdata` user.

```bash
cd /usr/libexec/netdata/plugins.d/
sudo su -s /bin/bash netdata
```

Now you can manually run the `bind_rndc` module in debug mode:

```bash
./python.d.plugin bind_rndc debug trace
```

