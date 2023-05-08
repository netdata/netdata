# Troubleshoot Agent-Cloud connectivity issues

Learn how to troubleshoot connectivity issues leading to agents not appearing at all in Netdata Cloud, or 
appearing with a status other than `live`.

After installing an agent with the claiming token provided by Netdata Cloud, you should see charts from that node on 
Netdata Cloud within seconds. If you don't see charts, check if the node appears in the list of nodes 
(Nodes tab, top right Node filter, or Manage Nodes screen). If your node does not appear in the list, or it does appear with a status other than `live`, this guide will help you troubleshoot what's happening.

 The most common explanation for connectivity issues usually falls into one of the following three categories:

- If the node does not appear at all in Netdata Cloud, [the claiming process was unsuccessful](#the-claiming-process-was-unsuccessful). 
- [The kickstart script auto-claimed the Agent but there was no error message displayed](#the-kickstart-script-auto-claimed-the-agent-but-there-was-no-error-message-displayed)
  - [Claiming on an older, deprecated version of the Agent](#claiming-on-an-older-deprecated-version-of-the-agent)
  - [Network issues while connecting to the Cloud](#network-issues-while-connecting-to-the-cloud)
    - [Verify that your IP is whitelisted from Netdata Cloud](#verify-that-your-ip-is-whitelisted-from-netdata-cloud)
    - [Make sure that your node has internet connectivity and can resolve network domains](#make-sure-that-your-node-has-internet-connectivity-and-can-resolve-network-domains)

## The claiming process was unsuccessful

If the claiming process fails, a **node does not appear at all in Netdata Cloud**. 
The possible causes differ between kickstart installations and Docker installations. 
However, first ensure that you **use the newest possible stable or nightly version of the agent**.

### Using kickstart.sh

Claiming is done by executing `netdata-claim.sh`, a script that is usually located under `${INSTALL_PREFIX}/netdata/usr/sbin/netdata-claim.sh`. Possible error conditions we have identified are:
- No script found at all in any of our search paths.
- The path where the claiming script should be does not exist.
- The path exists, but is not a file.
- The path is a file, but is not executable.
Check the output of the kickstart script for any reported errors claiming and verify that the claiming script exists 
and can be executed. 

### Using Docker

First verify that the NETDATA_CLAIM_TOKEN parameter is correctly configured and then check for any errors during
initialization of the container. 

The most common issue we have seen claiming nodes in Docker is [running on older hosts with seccomp enabled](https://github.com/netdata/netdata/blob/master/claim/README.md#known-issues-on-older-hosts-with-seccomp-enabled).

## The kickstart script auto-claimed the Agent but there was no error message displayed

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

> ### Note
>
> In some cases a simple restart of the Agent can fix the issue.  
> Read more about [Starting, Stopping and Restarting the Agent](https://github.com/netdata/netdata/blob/master/docs/configure/start-stop-restart.md).


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

    > ### Info
    >
    > There will be cases in which the firewall restricts network access. In those cases, you need to whitelist `api.netdata.cloud` and `mqtt.netdata.cloud` domains to be able to see your nodes in Netdata Cloud.  
    > If you can't whitelist domains in your firewall, you can whitelist the IPs that the above command will produce, but keep in mind that they can change without any notice.
