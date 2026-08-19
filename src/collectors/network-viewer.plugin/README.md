# network-viewer.plugin

This collector directory contains three integrations that share the same plugin target:

- [Network Connections](integrations/network_connections.md) on Linux
- [TCP Stack](integrations/tcp_stack.md) on Windows
- [UDP Stack](integrations/udp_stack.md) on Windows

## Overview

`network-viewer.plugin` builds the Linux socket-table viewer on Linux and the TCP/UDP perflib collectors on Windows.

## Default Configuration

The Linux `Network Connections` integration has no collector-specific options.
The Windows `TCP Stack` and `UDP Stack` integrations have no collector-specific options.
