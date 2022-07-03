---
title: "Troubleshooting Agent with Cloud connection issues"
description: "A simple guide to troubleshoot occurances where the Agent is showing as offline after claiming"
# image: /img/seo/guides/troubleshoot/monitor-debug-applications-ebpf.png
custom_edit_url: https://github.com/netdata/netdata/edit/master/guides/troubleshoot/troubleshooting-agent-not-connecting-to-cloud.md
---

Sometimes, when claiming a node, it might not show up as online in Netdata Cloud.  
The occurances triggering this behavior might be:

- Agent requiring a restart

- Claiming was executed on an agent with an old version, that was deprecated with the new architecture.

- Network or other temporary issue connecting to the Cloud.

## Agent requiring a restart

The case might just be restarting your Agent, which you can see how to do so [here](https://learn.netdata.cloud/docs/configure/start-stop-restart).

## Claiming on an older, deprecated version of the Agent

With the introduction of our new architecture, Agents running versions lower than v1.32.0 can face this problem, so we reccomend to [update the Netdata Agent](https://learn.netdata.cloud/docs/agent/packaging/installer/update).

## Network or other temporary issue connecting to the Cloud

If none of the above work, please make sure that your node has an internet connection and that the [Agent is running](https://learn.netdata.cloud/docs/configure/start-stop-restart).
