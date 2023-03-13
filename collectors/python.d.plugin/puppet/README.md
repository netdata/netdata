<!--
title: "Puppet monitoring with Netdata"
custom_edit_url: "https://github.com/netdata/netdata/edit/master/collectors/python.d.plugin/puppet/README.md"
sidebar_label: "Puppet"
learn_status: "Published"
learn_topic_type: "References"
learn_rel_path: "Integrations/Monitor/Provisioning tools"
-->

# Puppet collector

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

Edit the `python.d/puppet.conf` configuration file using `edit-config` from the Netdata [config
directory](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md), which is typically at `/etc/netdata`.

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




### Troubleshooting

To troubleshoot issues with the `puppet` module, run the `python.d.plugin` with the debug option enabled. The 
output will give you the output of the data collection job or error messages on why the collector isn't working.

First, navigate to your plugins directory, usually they are located under `/usr/libexec/netdata/plugins.d/`. If that's 
not the case on your system, open `netdata.conf` and look for the setting `plugins directory`. Once you're in the 
plugin's directory, switch to the `netdata` user.

```bash
cd /usr/libexec/netdata/plugins.d/
sudo su -s /bin/bash netdata
```

Now you can manually run the `puppet` module in debug mode:

```bash
./python.d.plugin puppet debug trace
```

