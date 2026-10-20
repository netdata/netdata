# network-viewer.plugin

This collector directory contains two integrations that share the same plugin target:

- [Network Connections](integrations/network_connections.md) on Linux, FreeBSD, macOS, and Windows
- [Windows Network Protocols](integrations/windows_network_protocols.md) on Windows

## Overview

`network-viewer.plugin` builds the shared network-connections and topology
viewers on Linux/FreeBSD/macOS/Windows and the TCP/UDP perflib collectors on
Windows.

## Default Configuration

The `Network Connections` integration has no collector-specific options.
The Windows `Network Protocols` integration has no collector-specific options.
