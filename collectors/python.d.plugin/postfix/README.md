<!--
title: "Postfix monitoring with Netdata"
custom_edit_url: "https://github.com/netdata/netdata/edit/master/collectors/python.d.plugin/postfix/README.md"
sidebar_label: "Postfix"
learn_status: "Published"
learn_topic_type: "References"
learn_rel_path: "Integrations/Monitor/Webapps"
-->

# Postfix collector

Monitors MTA email queue statistics using [postqueue](http://www.postfix.org/postqueue.1.html) tool.

The collector executes  `postqueue -p` to get Postfix queue statistics.

## Requirements

Postfix has internal access controls that limit activities on the mail queue. By default, all users are allowed to view
the queue. If your system is configured with stricter access controls, you need to grant the `netdata` user access to
view the mail queue. In order to do it, add `netdata` to `authorized_mailq_users` in the `/etc/postfix/main.cf` file.

See the `authorized_mailq_users` setting in
the [Postfix documentation](https://www.postfix.org/postconf.5.html) for more details.

## Charts

It produces only two charts:

1. **Postfix Queue Emails**

    - emails

2. **Postfix Queue Emails Size** in KB

    - size

## Configuration

Configuration is not needed.
### Troubleshooting

To troubleshoot issues with the `postfix` module, run the `python.d.plugin` with the debug option enabled. The 
output will give you the output of the data collection job or error messages on why the collector isn't working.

First, navigate to your plugins directory, usually they are located under `/usr/libexec/netdata/plugins.d/`. If that's 
not the case on your system, open `netdata.conf` and look for the setting `plugins directory`. Once you're in the 
plugin's directory, switch to the `netdata` user.

```bash
cd /usr/libexec/netdata/plugins.d/
sudo su -s /bin/bash netdata
```

Now you can manually run the `postfix` module in debug mode:

```bash
./python.d.plugin postfix debug trace
```

