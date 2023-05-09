# Troubleshoot Agent-Cloud connectivity issues

Learn how to troubleshoot connectivity issues leading to agents not appearing at all in Netdata Cloud, or 
appearing with a status other than `live`.

After installing an agent with the claiming token provided by Netdata Cloud, you should see charts from that node on 
Netdata Cloud within seconds. If you don't see charts, check if the node appears in the list of nodes 
(Nodes tab, top right Node filter, or Manage Nodes screen). If your node does not appear in the list, or it does appear with a status other than "Live", this guide will help you troubleshoot what's happening.

 The most common explanation for connectivity issues usually falls into one of the following three categories:

- If the node does not appear at all in Netdata Cloud, [the claiming process was unsuccessful](#the-claiming-process-was-unsuccessful). 
- If the node appears as in Netdata Cloud, but is in the "Unseen" state, [the Agent was claimed but can not connect](#the-agent-was-claimed-but-can-not-connect).
- If the node appears as in Netdata Cloud as "Offline" or "Stale", it is a [previously connected agent that can no longer connect](#previously-connected-agent-that-can-no-longer-connect).

## The claiming process was unsuccessful

If the claiming process fails, the node will not appear at all in Netdata Cloud. 

First ensure that you:
- Use the newest possible stable or nightly version of the agent (at least v1.32).
- Your node can successfully issue an HTTPS request to https://api.netdata.cloud 

Other possible causes differ between kickstart installations and Docker installations. 

### Verify your node can access Netdata Cloud

If you run either `curl` or `wget` to do an HTTPS request to https://api.netdata.cloud, you should get 
back a 404 response. If you do not, check your network connectivity, domain resolution,  
and firewall settings for outbound connections. 

If your firewall is configured to completely prevent outbound connections, you need to whitelist `api.netdata.cloud` and `mqtt.netdata.cloud`.  If you can't whitelist domains in your firewall, you can whitelist the IPs that the hostnames resolve to, but keep in mind that they can change without any notice.

If you use an outbound proxy, you need to [take some extra steps]( https://github.com/netdata/netdata/blob/master/claim/README.md#connect-through-a-proxy).

### Troubleshoot claiming with kickstart.sh

Claiming is done by executing `netdata-claim.sh`, a script that is usually located under `${INSTALL_PREFIX}/netdata/usr/sbin/netdata-claim.sh`. Possible error conditions we have identified are:
- No script found at all in any of our search paths.
- The path where the claiming script should be does not exist.
- The path exists, but is not a file.
- The path is a file, but is not executable.
Check the output of the kickstart script for any reported errors claiming and verify that the claiming script exists 
and can be executed. 

### Troubleshoot claiming with Docker

First verify that the NETDATA_CLAIM_TOKEN parameter is correctly configured and then check for any errors during
initialization of the container. 

The most common issue we have seen claiming nodes in Docker is [running on older hosts with seccomp enabled](https://github.com/netdata/netdata/blob/master/claim/README.md#known-issues-on-older-hosts-with-seccomp-enabled).

## The Agent was claimed but can not connect

Agents that appear on the cloud with state "Unseen" have successfully been claimed, but have never
been able to successfully establish an ACLK connection. 

Agents that appear with state "Offline" or "Stale" were able to connect at some point, but are currently not
connected. The difference between the two is that "Stale" nodes had some of their data replicated to a 
parent node that is still connected. 

### Verify that the agent is running

#### Troubleshoot connection establishment with kickstart.sh

The kickstart script will install/update your Agent and then try to claim the node to the Cloud 
(if tokens are provided). To complete the second part, the Agent must be running. In some platforms, 
the Netdata service cannot be enabled by default and you must do it manually, using the following steps:

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

#### Troubleshoot connection establishment with Docker

If a Netdata container exits or is killed before it properly starts, it may be able to complete the claiming
process, but not have enough time to establish the ACLK connection. 

### Verify that your firewall allows websockets

The agent initiates an SSL connection to `api.netdata.cloud` and then upgrades that connection to use secure 
websockets. Some firewalls completely prevent the use of websockets, even for outbound connections.

## Previously connected agent that can no longer connect

The states "Offline" and "Stale" suggest that the agent was able to connect at some point in the past, but
that it is currently not connected.

### Verify that network connectivity is still possible

Verify that you can still issue HTTPS requests to api.netdata.cloud and that no firewall or proxy changes were made. 

### Verify that the claiming info is persisted

If you use Docker, verify that the contents of `/var/lib/netdata` are preserved across container restarts, using a persistent volume. 

### Verify that the claiming info is not cloned

A relatively common case we have seen especially with VMs is two or more nodes sharing the same credentials. 
This happens if you claim a node in a VM and then create an image based on that node. Netdata can't properly
work this way, as we have unique node identification information under `/var/lib/netdata`.

### Verify that your IP is not blocked by Netdata Cloud

Most of the nodes change IPs dynamically. It is possible that your current IP has been restricted from accessing `api.netdata.cloud` due to security concerns, usually because it was spamming Netdata Coud with too many
failed requests (old versions of the agent).

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
