<!--
title: "Start, stop and restart Agent"
sidebar_label: "Start, stop and restart Agent"
custom_edit_url: "https://github.com/netdata/netdata/blob/master/docs/tasks/general-configuration/start-stop-and-restart-agent.md"
sidebar_position: 1
learn_status: "Published"
learn_topic_type: "Tasks"
learn_rel_path: "Operations"
learn_docs_purpose: "Instructions on how to Start, Stop and Restart the Netdata Agent"
-->

import Tabs from '@theme/Tabs';
import TabItem from '@theme/TabItem';
import Admonition from '@theme/Admonition';

Upon installing the Netdata Agent, the [daemon](https://github.com/netdata/netdata/blob/master/daemon/README.md) is
configured to start at boot and stop and restart/shutdown.

Keep in mind that you will most often need to _restart_ the Agent to load new or edited configuration files. Health
configuration files are the only exception, as they can be reloaded without restarting the entire Agent.

:::caution
Stopping or restarting the Agent will cause gaps in stored metrics until the `netdata` process initiates collectors and
the database engine.
:::

## Prerequisites

- A node with the Agent installed

## Steps

:::tip
`systemctl` is the recommended way to start, stop, or restart the Netdata daemon.  
If `systemctl` fails, or you know that you're using a non-systemd system, try using the `service` command.
Lastly, you can also use the `init.d` command for this operation.
:::

### Starting the Agent

You can start the Agent with one of the following options:

<Tabs groupId="choice">
<TabItem value="systemctl" label=<code>systemctl</code> default>

```bash
sudo systemctl start netdata
```

</TabItem>
<TabItem value="service" label=<code>service</code>>

```bash
sudo service netdata start
```

</TabItem>
<TabItem value="init.d" label=<code>init.d</code>>

```bash
sudo /etc/init.d/netdata start
```

</TabItem>
</Tabs>


You can also use the `netdata` command, typically located at `/usr/sbin/netdata`, to start the Netdata daemon.

```bash
sudo netdata
```

:::tip
If you start the daemon with the `netdata` command, close it with:

```bash
sudo killall netdata
```

:::

#### Expected result

After you start the Agent, you will be able to see its status by running one of the commands below:

<Tabs groupId="choice">
<TabItem value="systemctl" label=<code>systemctl</code> default>

```bash
systemctl status netdata
```

</TabItem>
<TabItem value="service" label=<code>service</code>>

```bash
service netdata status
```

</TabItem>
<TabItem value="init.d" label=<code>init.d</code>>

```bash
/etc/init.d/netdata status
```

</TabItem>
</Tabs>


If you successfully started the Agent, the status is going to report:

```bash
...
Active: active (running)
...
```

### Stopping the Agent

You can stop the Agent with one of the following options:

<Tabs groupId="choice">
<TabItem value="systemctl" label=<code>systemctl</code> default>

```bash
sudo systemctl stop netdata
```

</TabItem>
<TabItem value="service" label=<code>service</code>>

```bash
sudo service netdata stop
```

</TabItem>
<TabItem value="init.d" label=<code>init.d</code>>

```bash
sudo /etc/init.d/netdata stop
```

</TabItem>
</Tabs>


If you used the `netdata` command to start the daemon, you can close it with:

```bash
sudo killall netdata
```

Last but not least the Agent also comes with a [CLI tool](https://github.com/netdata/netdata/blob/master/cli/README.md)
capable of performing shutdowns:

```bash
sudo netdatacli shutdown-agent
```

#### Expected result

After you stop the Agent, you will be able to see its status by running one of the commands below:

<Tabs groupId="choice">
<TabItem value="systemctl" label=<code>systemctl</code> default>

```bash
systemctl status netdata
```

</TabItem>
<TabItem value="service" label=<code>service</code>>

```bash
service netdata status
```

</TabItem>
<TabItem value="init.d" label=<code>init.d</code>>

```bash
/etc/init.d/netdata status
```

</TabItem>
</Tabs>


If you successfully stopped the Agent, the status is going to report:

```bash
...
Active: inactive (dead)
...
```

### Reload health configuration

You do not need to restart the Agent between changes to health configuration files, such as specific health entities.

Instead, use `netdatacli` and the `reload-health` option to prevent gaps in metrics collection:

```bash
sudo netdatacli reload-health
```

If `netdatacli` doesn't work on your system, send a `SIGUSR2` signal to the daemon, which reloads health configuration
without restarting the entire process.

```bash
killall -USR2 netdata
```

### Restarting the Agent

You can restart the Agent with one of the following options:

<Tabs groupId="choice">
<TabItem value="systemctl" label=<code>systemctl</code> default>

```bash
sudo systemctl restart netdata
```

</TabItem>
<TabItem value="service" label=<code>service</code>>

```bash
sudo service netdata restart
```

</TabItem>
<TabItem value="init.d" label=<code>init.d</code>>

```bash
sudo /etc/init.d/netdata restart
```

</TabItem>
</Tabs>

#### Expected result

After you restart the Agent, you will be able to see its status by running one of the commands below:

<Tabs groupId="choice">
<TabItem value="systemctl" label=<code>systemctl</code> default>

```bash
systemctl status netdata
```

</TabItem>
<TabItem value="service" label=<code>service</code>>

```bash
service netdata status
```

</TabItem>
<TabItem value="init.d" label=<code>init.d</code>>

```bash
/etc/init.d/netdata status
```

</TabItem>
</Tabs>


If you successfully restarted the Agent, the status is going to report:

```bash
...
Active: active (running)
...
```

## Further actions

### Force stop stalled or unresponsive `netdata` processes

In rare cases, the Agent may stall or not properly close sockets, preventing a new process from starting. In these
cases, try the following three commands _(you can replace the `systemctl` command with your preferred command to stop
the Agent)_:

```bash
sudo systemctl stop netdata
sudo killall netdata
ps aux| grep netdata
```

The output of `ps aux` should show no `netdata` or associated processes running. You can now start the Agent again
with `service netdata start`, or the appropriate method for your system.

## Related topics

1. [Netdata Daemon Reference](https://github.com/netdata/netdata/blob/master/daemon/README.md)
