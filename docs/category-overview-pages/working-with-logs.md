# Working with Logs

This section talks about ways Netdata collects and visualizes logs.

The main Netdata plugin responsible for handling logs is the [`systemd journal` plugin](/src/collectors/systemd-journal.plugin/). [`log2journal`](/src/collectors/log2journal/README.md) and [`systemd-cat-native`](/src/libnetdata/log/systemd-cat-native.md) are tools used by that plugin in order to convert structured log files into `systemd-journal` entries.

You can also find useful guides on how to set up log centralization points in the [Observability Cetralization Points](/docs/observability-centralization-points/README.md) section of our docs.
