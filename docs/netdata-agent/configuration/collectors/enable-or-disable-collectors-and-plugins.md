# Enable or Disable Collectors and Plugins

By default, most Collectors and Plugins are enabled, but you might want to disable something specific for optimization purposes.

Using [`edit-config`](/docs/netdata-agent/configuration/README.md#edit-a-configuration-file-using-edit-config), open `netdata.conf` and scroll down to the `[plugins]` section.

To disable a plugin, uncomment it and set the value to `no`. For example, to explicitly keep the `proc` and `go.d` plugins enabled while disabling `python.d` and `charts.d`, you would do:

```text
[plugins]
    proc = yes
    python.d = no
    charts.d = no
    go.d = yes
```

Disable specific collectors by opening their respective plugin configuration files, uncommenting the line for the collector, and setting its value to `no`.

```bash
sudo ./edit-config go.d.conf
```

For example, to disable a few Go collectors:

```text
modules:
   adaptec_raid: no
   activemq: no
   ap: no
```

After you make your changes, restart the Agent with the [appropriate method](/docs/netdata-agent/start-stop-restart.md) for your system.
