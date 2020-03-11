<!--
---
title: "Puppet monitoring with Netdata"
custom_edit_url: https://github.com/netdata/netdata/edit/master/collectors/python.d.plugin/puppet/README.md
---
-->

# Puppet monitoring with Netdata

Monitor status of Puppet Server and Puppet DB.

Following charts are drawn:

1.  **JVM Heap**

    -   committed (allocated from OS)
    -   used (actual use)

2.  **JVM Non-Heap**

    -   committed (allocated from OS)
    -   used (actual use)

3.  **CPU Usage**

    -   execution
    -   GC (taken by garbage collection)

4.  **File Descriptors**

    -   max
    -   used

## Configuration

Edit the `python.d/puppet.conf` configuration file using `edit-config` from the your agent's [config
directory](../../../docs/step-by-step/step-04.md#find-your-netdataconf-file), which is typically at `/etc/netdata`.

```bash
cd /etc/netdata   # Replace this path with your Netdata config directory, if different
sudo ./edit-config python.d/puppet.conf
```

```yaml
puppetdb:
    url: 'https://fqdn.example.com:8081'
    tls_cert_file: /path/to/client.crt
    tls_key_file: /path/to/client.key
    autodetection_retry: 1

puppetserver:
    url: 'https://fqdn.example.com:8140'
    autodetection_retry: 1
```

When no configuration is given, module uses `https://fqdn.example.com:8140`.

### notes

-   Exact Fully Qualified Domain Name of the node should be used.
-   Usually Puppet Server/DB startup time is VERY long. So, there should
    be quite reasonable retry count.
-   Secure PuppetDB config may require client certificate. Not applies
    to default PuppetDB configuration though.

---

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fcollectors%2Fpython.d.plugin%2Fpuppet%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
