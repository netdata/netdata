# network-viewer.plugin

This collector directory contains two integrations that share the same plugin target:

- [Network Connections](integrations/network_connections.md) on Linux
- [TCP Stack](integrations/tcp_stack.md) on Windows

## Overview

`network-viewer.plugin` builds the Linux socket-table viewer on Linux and the TCP perflib collector on Windows.

## Default Configuration

The Linux `Network Connections` integration has no collector-specific options.
The Windows `TCP Stack` integration uses the standard `netdata.conf` section shown in its integration guide.
