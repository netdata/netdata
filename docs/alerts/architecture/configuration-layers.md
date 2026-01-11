# 13.2 Configuration Layers

Netdata supports multiple configuration layers for health alerts. Understanding precedence rules helps in making modifications that take effect as intended.

## Stock Configuration Layer

Stock alerts are distributed with Netdata and reside in `/usr/lib/netdata/conf.d/health.d/`. These files are installed by the Netdata package and updated with each release. Modifying stock files is not recommended because changes are overwritten during upgrades.

Stock configurations define the default alert set. They are evaluated first, so any custom configuration with the same alert name overrides the stock definition.

## Custom Configuration Layer

Custom alerts reside in `/etc/netdata/health.d/`. Files in this directory take precedence over stock configurations for the same alert names.

An alert defined in `/etc/netdata/health.d/` with the same name as a stock alert replaces the stock definition entirely.

## Cloud Configuration Layer

Netdata Cloud can define alerts through the Alerts Configuration Manager. These Cloud-defined alerts take precedence over both stock and custom layers.

Cloud-defined alerts are stored remotely and synchronized to Agents on demand.

## Configuration File Merging

At startup and after configuration changes, Netdata merges all configuration layers into a single effective configuration.

Use `netdatacli health configuration` to view the effective merged configuration.