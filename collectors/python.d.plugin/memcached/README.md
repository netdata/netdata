<!--
title: "Memcached monitoring with Netdata"
custom_edit_url: "https://github.com/netdata/netdata/edit/master/collectors/python.d.plugin/memcached/README.md"
sidebar_label: "Memcached"
learn_status: "Published"
learn_topic_type: "References"
learn_rel_path: "Integrations/Monitor/Databases"
-->

# Memcached collector

Collects memory-caching system performance metrics. It reads server response to stats command ([stats interface](https://github.com/memcached/memcached/wiki/Commands#stats)).


1.  **Network** in kilobytes/s

    -   read
    -   written

2.  **Connections** per second

    -   current
    -   rejected
    -   total

3.  **Items** in cluster

    -   current
    -   total

4.  **Evicted and Reclaimed** items

    -   evicted
    -   reclaimed

5.  **GET** requests/s

    -   hits
    -   misses

6.  **GET rate** rate in requests/s

    -   rate

7.  **SET rate** rate in requests/s

    -   rate

8.  **DELETE** requests/s

    -   hits
    -   misses

9.  **CAS** requests/s

    -   hits
    -   misses
    -   bad value

10. **Increment** requests/s

    -   hits
    -   misses

11. **Decrement** requests/s

    -   hits
    -   misses

12. **Touch** requests/s

    -   hits
    -   misses

13. **Touch rate** rate in requests/s

    -   rate

## Configuration

Edit the `python.d/memcached.conf` configuration file using `edit-config` from the Netdata [config
directory](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md), which is typically at `/etc/netdata`.

```bash
cd /etc/netdata   # Replace this path with your Netdata config directory, if different
sudo ./edit-config python.d/memcached.conf
```

Sample:

```yaml
localtcpip:
  name     : 'local'
  host     : '127.0.0.1'
  port     : 24242
```

If no configuration is given, module will attempt to connect to memcached instance on `127.0.0.1:11211` address.




### Troubleshooting

To troubleshoot issues with the `memcached` module, run the `python.d.plugin` with the debug option enabled. The 
output will give you the output of the data collection job or error messages on why the collector isn't working.

First, navigate to your plugins directory, usually they are located under `/usr/libexec/netdata/plugins.d/`. If that's 
not the case on your system, open `netdata.conf` and look for the setting `plugins directory`. Once you're in the 
plugin's directory, switch to the `netdata` user.

```bash
cd /usr/libexec/netdata/plugins.d/
sudo su -s /bin/bash netdata
```

Now you can manually run the `memcached` module in debug mode:

```bash
./python.d.plugin memcached debug trace
```

