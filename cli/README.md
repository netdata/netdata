<!--
title: "Netdata CLI"
description: "The Netdata Agent includes a command-line experience for reloading health configuration, reopening log files, halting the daemon, and more."
custom_edit_url: "https://github.com/netdata/netdata/edit/master/cli/README.md"
sidebar_label: "Agent CLI"
learn_status: "Published"
learn_topic_type: "References"
learn_rel_path: "References"
-->

# Netdata CLI

You can see the commands `netdatacli` supports by executing it with `netdatacli` and entering `help` in
standard input. All commands are given as standard input to `netdatacli`.

The commands that a running netdata agent can execute are the following:

```sh
The commands are (arguments are in brackets):
help
    Show this help menu.
reload-health
    Reload health configuration.
reload-labels
    Reload all labels.
save-database
    Save internal DB to disk for database mode save.
reopen-logs
    Close and reopen log files.
shutdown-agent
    Cleanup and exit the netdata agent.
fatal-agent
    Log the state and halt the netdata agent.
reload-claiming-state
    Reload agent claiming state from disk.
ping
    Return with 'pong' if agent is alive.
aclk-state [json]
    Returns current state of ACLK and Cloud connection. (optionally in json)
```

Those commands are the same that can be sent to netdata via [signals](https://github.com/netdata/netdata/blob/master/daemon/README.md#command-line-options).


