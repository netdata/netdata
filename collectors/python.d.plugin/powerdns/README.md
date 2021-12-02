<!--
title: "PowerDNS monitoring with Netdata"
custom_edit_url: https://github.com/netdata/netdata/edit/master/collectors/python.d.plugin/powerdns/README.md
sidebar_label: "PowerDNS"
-->

# PowerDNS monitoring with Netdata

Monitors authoritative server and recursor statistics.

Powerdns charts:

1.  **Queries and Answers**

    -   udp-queries
    -   udp-answers
    -   tcp-queries
    -   tcp-answers

2.  **Cache Usage**

    -   query-cache-hit
    -   query-cache-miss
    -   packetcache-hit
    -   packetcache-miss

3.  **Cache Size**

    -   query-cache-size
    -   packetcache-size
    -   key-cache-size
    -   meta-cache-size

4.  **Latency**

    -   latency

 Powerdns Recursor charts:

1.  **Questions In**

    -   questions
    -   ipv6-questions
    -   tcp-queries

2.  **Questions Out**

    -   all-outqueries
    -   ipv6-outqueries
    -   tcp-outqueries
    -   throttled-outqueries

3.  **Answer Times**

    -   answers-slow
    -   answers0-1
    -   answers1-10
    -   answers10-100
    -   answers100-1000

4.  **Timeouts**

    -   outgoing-timeouts
    -   outgoing4-timeouts
    -   outgoing6-timeouts

5.  **Drops**

    -   over-capacity-drops

6.  **Cache Usage**

    -   cache-hits
    -   cache-misses
    -   packetcache-hits
    -   packetcache-misses

7.  **Cache Size**

    -   cache-entries
    -   packetcache-entries
    -   negcache-entries

## Configuration

Edit the `python.d/powerdns.conf` configuration file using `edit-config` from the Netdata [config
directory](/docs/configure/nodes.md), which is typically at `/etc/netdata`.

```bash
cd /etc/netdata   # Replace this path with your Netdata config directory, if different
sudo ./edit-config python.d/powerdns.conf
```

```yaml
local:
  name     : 'local'
  url     : 'http://127.0.0.1:8081/api/v1/servers/localhost/statistics'
  header   :
    X-API-Key: 'change_me'
```

---

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fcollectors%2Fpython.d.plugin%2Fpowerdns%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
