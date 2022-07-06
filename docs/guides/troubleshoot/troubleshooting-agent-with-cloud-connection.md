<!--
title: "Troubleshooting Agent with Cloud connection issues"
description: "A simple guide to troubleshoot occurances where the Agent is showing as offline after claiming."
custom_edit_url: https://github.com/netdata/netdata/edit/master/guides/troubleshoot/troubleshooting-agent-not-connecting-to-cloud.md
-->

# Troubleshooting Agent with Cloud connection issues

Sometimes, when claiming a node, it might not show up as online in Netdata Cloud.  
The occurances triggering this behavior might be:

- [The claiming script failed](#the-claiming-script-failed)
- [Claiming on an older, deprecated version of the Agent](#claiming-on-an-older-deprecated-version-of-the-agent)
- [Network issues while connecting to the Cloud](#network-issues-while-connecting-to-the-cloud)

## The claiming script failed

### Make sure the Agent is running

Check if the Agent is running:

```bash
systemctl status netdata
```

The expected output should contain info like this:

```bash
Active: active (running) since Wed 2022-07-06 12:25:02 EEST; 1h 40min ago
```

:::note

The Agent must be running for the claiming to work.

:::

If the Agent is not running, enable the service:

```bash
sudo systemctl enable netdata 
```

and then start the Agent:

```bash
sudo systemctl start netdata 
```

:::info

Read more about [Starting, Stopping and Restarting the Agent](https://learn.netdata.cloud/docs/configure/start-stop-restart).

:::

## Claiming on an older, deprecated version of the Agent

Make sure that you are using the latest version of Netdata if you are using the [Claiming script](https://learn.netdata.cloud/docs/agent/claim#claiming-script).

With the introduction of our new architecture, Agents running versions lower than v1.32.0 can face claiming problems, so we reccomend to [update the Netdata Agent](https://learn.netdata.cloud/docs/agent/packaging/installer/update).

## Network issues while connecting to the Cloud

### Check your IP

It is possible that your IP might be banned from `app.netdata.cloud`, for security reasons.

To check this, run:

```bash
sudo netdatacli aclk-state 
```

The output will contain a line indicating if the IP is banned from `app.netdata.cloud`:

```bash
Banned By Cloud: No
```

If your node's ip is banned, you can:

- Contact our team to whitelist your IP by sumbiting a ticket at [Netdata Community](https://community.netdata.cloud/)
- Change your node's IP

### Make sure that you have an internet connection

Firstly, check that you have internet connection by pinging well known hosts:

```bash
ping 8.8.8.8
```

or

```bash
ping 1.1.1.1
```

:::tip

Exit the `ping` command by pressing `Ctrl+C`

:::

### Check DNS resolution

You can check your DNS resolution by running:

```bash
host app.netdata.cloud
```

The expected output should be something like this:

```bash
app.netdata.cloud is an alias for main-ingress-545609a41fcaf5d6.elb.us-east-1.amazonaws.com.
main-ingress-545609a41fcaf5d6.elb.us-east-1.amazonaws.com has address 54.198.178.11
main-ingress-545609a41fcaf5d6.elb.us-east-1.amazonaws.com has address 44.207.131.212
main-ingress-545609a41fcaf5d6.elb.us-east-1.amazonaws.com has address 44.196.50.41
```

If the command fails, you can whitelist the `app.netdata.cloud` domain from the node's firewall restrictions.

:::tip

If you can't whitelist domains from your firewall, you can whitelist the IPs that the above mentioned command will produce, but keep in mind that they can change at anytime without notice.

:::
