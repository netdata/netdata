<!--
title: "Example module in Python"
custom_edit_url: "https://github.com/netdata/netdata/edit/master/collectors/python.d.plugin/example/README.md"
sidebar_label: "Example module in Python"
learn_status: "Published"
learn_topic_type: "References"
learn_rel_path: "Integrations/Monitor/Mock Collectors"
-->

# Example module in Python

You can add custom data collectors using Python.

Netdata provides an [example python data collection module](https://github.com/netdata/netdata/tree/master/collectors/python.d.plugin/example).

If you want to write your own collector, read our [writing a new Python module](https://github.com/netdata/netdata/blob/master/collectors/python.d.plugin/README.md#how-to-write-a-new-module) tutorial.


### Troubleshooting

To troubleshoot issues with the `example` module, run the `python.d.plugin` with the debug option enabled. The 
output will give you the output of the data collection job or error messages on why the collector isn't working.

First, navigate to your plugins directory, usually they are located under `/usr/libexec/netdata/plugins.d/`. If that's 
not the case on your system, open `netdata.conf` and look for the setting `plugins directory`. Once you're in the 
plugin's directory, switch to the `netdata` user.

```bash
cd /usr/libexec/netdata/plugins.d/
sudo su -s /bin/bash netdata
```

Now you can manually run the `example` module in debug mode:

```bash
./python.d.plugin example debug trace
```

