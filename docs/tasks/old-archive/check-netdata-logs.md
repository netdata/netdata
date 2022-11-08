<!--
title: "Check netdata logs"
sidebar_label: "Check netdata logs"
custom_edit_url: "https://github.com/netdata/netdata/blob/master/docs/tasks/miscellaneous/check-netdata-logs.md"
learn_status: "Unpublished"
learn_topic_type: "Tasks"
learn_rel_path: "Operations"
learn_docs_purpose: "Instructions on how to inspect the logs"
-->

Netdata uses 3 log files, `error.log`, `access.log` and `debug.log`; you can read more at
the [Daemon Reference](https://github.com/netdata/netdata/blob/master/daemon/README.md) documentation.

Log files are stored in `/var/log/netdata/` by default.

## Prerequisites

To check Netdata logs, you will need the following:

- A node with the Agent installed 
- Terminal access to that node

## Steps

To access the logs of your Agent you can:

- See the logs in a page like manner:
    ```bash
    less /var/log/netdata/error.log
    ```

- Search for a specific keyword, using:
    ```bash
    grep -w <keyword> /var/log/netdata/error.log
    ```

## Related topics

### Related refernces

- [Daemon](https://github.com/netdata/netdata/blob/master/daemon/README.md) 

