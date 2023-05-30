<!--
title: "Exim monitoring with Netdata"
custom_edit_url: "https://github.com/netdata/netdata/edit/master/collectors/python.d.plugin/exim/README.md"
sidebar_label: "Exim"
learn_status: "Published"
learn_topic_type: "References"
learn_rel_path: "Integrations/Monitor/Webapps"
-->

# Exim collector

Simple module executing `exim -bpc` to grab exim queue.
This command can take a lot of time to finish its execution thus it is not recommended to run it every second.

## Requirements

The module uses the `exim` binary, which can only be executed as root by default. We need to allow other users to `exim` binary. We solve that adding `queue_list_requires_admin` statement in exim configuration and set to `false`, because it is `true` by default. On many Linux distributions, the default location of `exim` configuration is in `/etc/exim.conf`.

1. Edit the `exim` configuration with your preferred editor and add:
`queue_list_requires_admin = false`
2. Restart `exim` and Netdata

*WHM (CPanel) server*

On a WHM server, you can reconfigure `exim` over the WHM interface with the following steps.

1. Login to WHM
2. Navigate to Service Configuration --> Exim Configuration Manager --> tab Advanced Editor
3. Scroll down to the button **Add additional configuration setting** and click on it.
4. In the new dropdown which will appear above we need to find and choose:
`queue_list_requires_admin` and set to `false` 
5. Scroll to the end and click the **Save** button.

It produces only one chart:

1.  **Exim Queue Emails**

    -   emails

Configuration is not needed.




### Troubleshooting

To troubleshoot issues with the `exim` module, run the `python.d.plugin` with the debug option enabled. The 
output will give you the output of the data collection job or error messages on why the collector isn't working.

First, navigate to your plugins directory, usually they are located under `/usr/libexec/netdata/plugins.d/`. If that's 
not the case on your system, open `netdata.conf` and look for the setting `plugins directory`. Once you're in the 
plugin's directory, switch to the `netdata` user.

```bash
cd /usr/libexec/netdata/plugins.d/
sudo su -s /bin/bash netdata
```

Now you can manually run the `exim` module in debug mode:

```bash
./python.d.plugin exim debug trace
```

