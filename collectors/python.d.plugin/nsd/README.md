<!--
title: "NSD monitoring with Netdata"
custom_edit_url: "https://github.com/netdata/netdata/edit/master/collectors/python.d.plugin/nsd/README.md"
sidebar_label: "NSD"
learn_status: "Published"
learn_topic_type: "References"
learn_rel_path: "Integrations/Monitor/Networking"
-->

# NSD collector

Uses the `nsd-control stats_noreset` command to provide `nsd` statistics.

## Requirements

-   Version of `nsd` must be 4.0+
-   Netdata must have permissions to run `nsd-control stats_noreset`

It produces:

1.  **Queries**

    -   queries

2.  **Zones**

    -   master
    -   slave

3.  **Protocol**

    -   udp
    -   udp6
    -   tcp
    -   tcp6

4.  **Query Type**

    -   A
    -   NS
    -   CNAME
    -   SOA
    -   PTR
    -   HINFO
    -   MX
    -   NAPTR
    -   TXT
    -   AAAA
    -   SRV
    -   ANY

5.  **Transfer**

    -   NOTIFY
    -   AXFR

6.  **Return Code**

    -   NOERROR
    -   FORMERR
    -   SERVFAIL
    -   NXDOMAIN
    -   NOTIMP
    -   REFUSED
    -   YXDOMAIN

Configuration is not needed.




### Troubleshooting

To troubleshoot issues with the `nsd` module, run the `python.d.plugin` with the debug option enabled. The 
output will give you the output of the data collection job or error messages on why the collector isn't working.

First, navigate to your plugins directory, usually they are located under `/usr/libexec/netdata/plugins.d/`. If that's 
not the case on your system, open `netdata.conf` and look for the setting `plugins directory`. Once you're in the 
plugin's directory, switch to the `netdata` user.

```bash
cd /usr/libexec/netdata/plugins.d/
sudo su -s /bin/bash netdata
```

Now you can manually run the `nsd` module in debug mode:

```bash
./python.d.plugin nsd debug trace
```

