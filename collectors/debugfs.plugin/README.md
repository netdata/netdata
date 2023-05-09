<!--
title: "DebugFS monitoring (debugfs.plugin)"
sidebar_label: "DebugFS monitoring "
custom_edit_url: "https://github.com/netdata/netdata/edit/master/collectors/debugfs.plugin/README.md"
learn_status: "Not Published"
learn_topic_type: "References"
learn_rel_path: "Integrations/Monitor/System metrics"
-->

# Debugfs 

According kernel official documentation, [debugfs](https://docs.kernel.org/filesystems/debugfs.html) is a simple way for
kernel developers to bring information to user ring. Some of these [information](https://github.com/netdata/netdata/issues/15001)
are also useful for overall people to understand memory management.

## Mount filesystem

The debugfs plugin collects data from `/sys/kernel/debug` that is not always present. To mount the filesystem you need
to run

```sh
# mount -t debugfs none /sys/kernel/debug
```

## Permissions

It is not possible to collect data from `/sys/kernel/debug` without special capabilites or permission. To overcome
this situation netdata modifies during installation the plugin permissions. 
