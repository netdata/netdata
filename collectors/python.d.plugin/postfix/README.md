<!--
title: "Postfix monitoring with Netdata"
custom_edit_url: "https://github.com/netdata/netdata/edit/master/collectors/python.d.plugin/postfix/README.md"
sidebar_label: "Postfix"
learn_status: "Published"
learn_topic_type: "References"
learn_rel_path: "Integrations/Monitor/Webapps"
-->

# Postfix monitoring with Netdata

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
