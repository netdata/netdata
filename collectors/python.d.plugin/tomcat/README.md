<!--
title: "Apache Tomcat monitoring with Netdata"
custom_edit_url: "https://github.com/netdata/netdata/edit/master/collectors/python.d.plugin/tomcat/README.md"
sidebar_label: "Tomcat"
learn_status: "Published"
learn_topic_type: "References"
learn_rel_path: "Integrations/Monitor/Webapps"
-->

# Apache Tomcat collector

Presents memory utilization of tomcat containers.

Charts:

1.  **Requests** per second

    -   accesses

2.  **Volume** in KB/s

    -   volume

3.  **Threads**

    -   current
    -   busy

4.  **JVM Free Memory** in MB

    -   jvm

## Configuration

Edit the `python.d/tomcat.conf` configuration file using `edit-config` from the Netdata [config
directory](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md), which is typically at `/etc/netdata`.

```bash
cd /etc/netdata   # Replace this path with your Netdata config directory, if different
sudo ./edit-config python.d/tomcat.conf
```

```yaml
localhost:
  name : 'local'
  url  : 'http://127.0.0.1:8080/manager/status?XML=true'
  user : 'tomcat_username'
  pass : 'secret_tomcat_password'
```

Without configuration, module attempts to connect to `http://localhost:8080/manager/status?XML=true`, without any credentials.
So it will probably fail.




### Troubleshooting

To troubleshoot issues with the `tomcat` module, run the `python.d.plugin` with the debug option enabled. The 
output will give you the output of the data collection job or error messages on why the collector isn't working.

First, navigate to your plugins directory, usually they are located under `/usr/libexec/netdata/plugins.d/`. If that's 
not the case on your system, open `netdata.conf` and look for the setting `plugins directory`. Once you're in the 
plugin's directory, switch to the `netdata` user.

```bash
cd /usr/libexec/netdata/plugins.d/
sudo su -s /bin/bash netdata
```

Now you can manually run the `tomcat` module in debug mode:

```bash
./python.d.plugin tomcat debug trace
```

