# Working with Logs

This section talks about the ways Netdata collects and visualizes logs.

The [systemd journal plugin](/src/collectors/systemd-journal.plugin) is the core Netdata component for reading systemd journal logs.

For structured logs, Netdata provides tools like [log2journal](/src/collectors/log2journal/README.md) and [systemd-cat-native](/src/libnetdata/log/systemd-cat-native.md) to convert them into compatible systemd journal entries.

You can also find useful guides on how to set up log centralization points in the [Observability Centralization Points](/docs/observability-centralization-points/README.md) section of our docs.
