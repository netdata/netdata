<!--
title: "Install Agent and add it to your space"
sidebar_label: "Install Agent and add it to your space"
custom_edit_url: "https://github.com/netdata/netdata/blob/master/docs/tasks/setup/deploy-agent-and-claim.md"
learn_status: "Published"
sidebar_position: "20"
learn_topic_type: "Tasks"
learn_rel_path: "Setup"
learn_docs_purpose: "Step by step instruction to deploy an Agent"
-->

To accurately monitor the health of your systems and applications, you need to know _immediately_ when there's something
strange going on. Netdata's alert and notification systems are essential to keeping you informed.

Netdata comes with hundreds of pre-configured alerts that don't require configuration. They were designed by our
community of system administrators to cover the most important parts of production systems, so, in many cases, you won't
need to edit them.

Luckily, Netdata's alert and notification systems are incredibly adaptable to your infrastructure's unique needs.
Many Netdata users don't need all the default alerts enabled. Instead of disabling any given alert, or even _all_
alerts, you can silence individual alerts by changing one line in a given health entity.

## Prerequisites

- A node with the Agent installed, and terminal access to that node

## Steps

All of Netdata's health configuration files are in Netdata's config directory, inside the `health.d/` directory. You can
edit them by following
the [Configure the Agent](https://github.com/netdata/netdata/blob/master/docs/tasks/general-configuration/configure-the-agent.md)
Task.

For example, lets take a look at the `health/cpu.conf` file.

```yaml
template: 10min_cpu_usage
  on: system.cpu
  os: linux
  hosts: *
  lookup: average -10m unaligned of user,system,softirq,irq,guest
    units: %
    every: 1m
      warn:
        $this > (($status >= $WARNING)  ? (75): (85))
      crit:
        $this > (($status == $CRITICAL) ? (85): (95))
    delay: down 15m multiplier 1.5 max 1h
      info: average cpu utilization for the last 10 minutes (excluding iowait, nice and steal)
        to: sysadmin
```

To silence this alert, change `sysadmin` to `silent`.

```yaml
      to: silent
```

:::tip
To make any changes to your health configuration live, you must reload Netdata's health monitoring system. To do that
without restarting all of Netdata, run `netdatacli reload-health` or `killall -USR2 netdata`.
:::
