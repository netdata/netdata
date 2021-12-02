<!--
title: "Nvidia GPU monitoring with Netdata"
custom_edit_url: https://github.com/netdata/netdata/edit/master/collectors/python.d.plugin/nvidia_smi/README.md
sidebar_label: "Nvidia GPUs"
-->

# Nvidia GPU monitoring with Netdata

Monitors performance metrics (memory usage, fan speed, pcie bandwidth utilization, temperature, etc.) using `nvidia-smi` cli tool.


## Requirements and Notes

-   You must have the `nvidia-smi` tool installed and your NVIDIA GPU(s) must support the tool. Mostly the newer high end models used for AI / ML and Crypto or Pro range, read more about [nvidia_smi](https://developer.nvidia.com/nvidia-system-management-interface).
-   You must enable this plugin, as its disabled by default due to minor performance issues:
    ```bash
    cd /etc/netdata   # Replace this path with your Netdata config directory, if different
    sudo ./edit-config python.d.conf
    ```
    Remove the '#' before nvidia_smi so it reads: `nvidia_smi: yes`.

-   On some systems when the GPU is idle the `nvidia-smi` tool unloads and there is added latency again when it is next queried. If you are running GPUs under constant workload this isn't likely to be an issue.
-   Currently the `nvidia-smi` tool is being queried via cli. Updating the plugin to use the nvidia c/c++ API directly should resolve this issue. See discussion here: <https://github.com/netdata/netdata/pull/4357>
-   Contributions are welcome.
-   Make sure `netdata` user can execute `/usr/bin/nvidia-smi` or wherever your binary is.
-   If `nvidia-smi` process [is not killed after netdata restart](https://github.com/netdata/netdata/issues/7143) you need to off `loop_mode`.
-   `poll_seconds` is how often in seconds the tool is polled for as an integer.

## Charts

It produces the following charts:

-   PCI Express Bandwidth Utilization in `KiB/s`
-   Fan Speed in `percentage`
-   GPU Utilization in `percentage`
-   Memory Bandwidth Utilization in `percentage`
-   Encoder/Decoder Utilization in `percentage`
-   Memory Usage in `MiB`
-   Temperature in `celsius`
-   Clock Frequencies in `MHz`
-   Power Utilization in `Watts`
-   Memory Used by Each Process in `MiB`
-   Memory Used by Each User in `MiB`
-   Number of User on GPU in `num`

## Configuration

Edit the `python.d/nvidia_smi.conf` configuration file using `edit-config` from the Netdata [config
directory](/docs/configure/nodes.md), which is typically at `/etc/netdata`.

```bash
cd /etc/netdata   # Replace this path with your Netdata config directory, if different
sudo ./edit-config python.d/nvidia_smi.conf
```

Sample:

```yaml
loop_mode    : yes
poll_seconds : 1
exclude_zero_memory_users : yes
```

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fcollectors%2Fpython.d.plugin%2Fnvidia_smi%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
