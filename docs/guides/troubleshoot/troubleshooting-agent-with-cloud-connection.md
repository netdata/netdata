<!--
title: "Troubleshoot Agent-Cloud connectivity issues"
sidebar_label: "Troubleshoot Agent-Cloud connectivity issues"
description: "A simple guide to troubleshoot occurrences where the Agent is showing as offline after claiming."
custom_edit_url: https://github.com/netdata/netdata/edit/master/docs/guides/troubleshoot/troubleshooting-agent-with-cloud-connection.md
learn_status: "Published"
learn_topic_type: "Tasks"
learn_rel_path: "Operations"
-->

# Troubleshoot Agent-Cloud connectivity issues

When you are claiming a node, you might not be able to immediately see it online in Netdata Cloud.  
This could be due to an error in the claiming process or a temporary outage of some services.

We identified some scenarios that might cause this delay and possible actions you could take to overcome each situation.

The most common explanation for the delay usually falls into one of the following three categories:

- [Troubleshoot Agent-Cloud connectivity issues](#troubleshoot-agent-cloud-connectivity-issues)
  - [The claiming process of the kickstart script was unsuccessful](#the-claiming-process-of-the-kickstart-script-was-unsuccessful)
    - [The kickstart script auto-claimed the Agent but there was no error message displayed](#the-kickstart-script-auto-claimed-the-agent-but-there-was-no-error-message-displayed)
  - [Claiming on an older, deprecated version of the Agent](#claiming-on-an-older-deprecated-version-of-the-agent)
  - [Network issues while connecting to the Cloud](#network-issues-while-connecting-to-the-cloud)
    - [Verify that your IP is whitelisted from Netdata Cloud](#verify-that-your-ip-is-whitelisted-from-netdata-cloud)
    - [Make sure that your node has internet connectivity and can resolve network domains](#make-sure-that-your-node-has-internet-connectivity-and-can-resolve-network-domains)

## The claiming process of the kickstart script was unsuccessful

Here, we will try to define some edge cases you might encounter when claiming a node.

### The kickstart script auto-claimed the Agent but there was no error message displayed

The kickstart script will install/update your Agent and then try to claim the node to the Cloud (if tokens are provided). To
complete the second part, the Agent must be running. In some platforms, the Netdata service cannot be enabled by default
and you must do it manually, using the following steps:

1. Check if the Agent is running:

    ```bash
    systemctl status netdata
    ```

    The expected output should contain info like this:

    ```bash
    Active: active (running) since Wed 2022-07-06 12:25:02 EEST; 1h 40min ago
    ```

2. Enable and start the Netdata Service.

    ```bash
    systemctl enable netdata
    systemctl start netdata
    ```

3. Retry the kickstart claiming process.

:::note

In some cases a simple restart of the Agent can fix the issue.  
Read more about [Starting, Stopping and Restarting the Agent](https://github.com/netdata/netdata/blob/master/docs/configure/start-stop-restart.md).

:::

## Claiming on an older, deprecated version of the Agent

Make sure that you are using the latest version of Netdata if you are using the [Claiming script](https://learn.netdata.cloud/docs/agent/claim#claiming-script).

With the introduction of our new architecture, Agents running versions lower than `v1.32.0` can face claiming problems, so we recommend you [update the Netdata Agent](https://github.com/netdata/netdata/blob/master/packaging/installer/UPDATE.md) to the latest stable version.

## Network issues while connecting to the Cloud

### Verify that your IP is whitelisted from Netdata Cloud

Most of the nodes change IPs dynamically. It is possible that your current IP has been restricted from accessing `api.netdata.cloud` due to security concerns.

To verify this:

1. Check the Agent's `aclk-state`.

    ```bash
    sudo netdatacli aclk-state | grep "Banned By Cloud"
    ```

    The output will contain a line indicating if the IP is banned from `api.netdata.cloud`:

    ```bash
    Banned By Cloud: yes
    ```

2. If your node's IP is banned, you can:

    - Contact our team to whitelist your IP by submitting a ticket in the [Netdata forum](https://community.netdata.cloud/)
    - Change your node's IP

### Make sure that your node has internet connectivity and can resolve network domains

1. Try to reach a well known host:

    ```bash
    ping 8.8.8.8
    ```
  
2. If you can reach external IPs, then check your domain resolution.

    ```bash
    host api.netdata.cloud
    ```

    The expected output should be something like this:

    ```bash
    api.netdata.cloud is an alias for main-ingress-545609a41fcaf5d6.elb.us-east-1.amazonaws.com.
    main-ingress-545609a41fcaf5d6.elb.us-east-1.amazonaws.com has address 54.198.178.11
    main-ingress-545609a41fcaf5d6.elb.us-east-1.amazonaws.com has address 44.207.131.212
    main-ingress-545609a41fcaf5d6.elb.us-east-1.amazonaws.com has address 44.196.50.41
    ```

    :::info

    There will be cases in which the firewall restricts network access. In those cases, you need to whitelist `api.netdata.cloud` and `mqtt.netdata.cloud` domains to be able to see your nodes in Netdata Cloud.  
    If you can't whitelist domains in your firewall, you can whitelist the IPs that the above command will produce, but keep in mind that they can change without any notice.

    :::
