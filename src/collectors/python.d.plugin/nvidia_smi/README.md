<!--
title: "Nvidia GPU monitoring with Netdata"
custom_edit_url: "https://github.com/netdata/netdata/edit/master/src/collectors/python.d.plugin/nvidia_smi/README.md"
sidebar_label: "nvidia_smi-python.d.plugin"
learn_status: "Published"
learn_topic_type: "References"
learn_rel_path: "Integrations/Monitor/Devices"
-->

# Nvidia GPU collector

Monitors performance metrics (memory usage, fan speed, pcie bandwidth utilization, temperature, etc.) using `nvidia-smi` cli tool.

## Requirements

-   The `nvidia-smi` tool installed and your NVIDIA GPU(s) must support the tool. Mostly the newer high end models used for AI / ML and Crypto or Pro range, read more about [nvidia_smi](https://developer.nvidia.com/nvidia-system-management-interface).
-   Enable this plugin, as it's disabled by default due to minor performance issues:
    ```bash
    cd /etc/netdata   # Replace this path with your Netdata config directory, if different
    sudo ./edit-config python.d.conf
    ```
    Remove the '#' before nvidia_smi so it reads: `nvidia_smi: yes`.
-   On some systems when the GPU is idle the `nvidia-smi` tool unloads and there is added latency again when it is next queried. If you are running GPUs under constant workload this isn't likely to be an issue.

If using Docker, see [Netdata Docker container with NVIDIA GPUs monitoring](https://github.com/netdata/netdata/tree/master/packaging/docker#with-nvidia-gpus-monitoring).  

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
directory](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md), which is typically at `/etc/netdata`.

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


### Troubleshooting

To troubleshoot issues with the `nvidia_smi` module, run the `python.d.plugin` with the debug option enabled. The 
output will give you the output of the data collection job or error messages on why the collector isn't working.

First, navigate to your plugins directory, usually they are located under `/usr/libexec/netdata/plugins.d/`. If that's 
not the case on your system, open `netdata.conf` and look for the setting `plugins directory`. Once you're in the 
plugin's directory, switch to the `netdata` user.

```bash
cd /usr/libexec/netdata/plugins.d/
sudo su -s /bin/bash netdata
```

Now you can manually run the `nvidia_smi` module in debug mode:

```bash
./python.d.plugin nvidia_smi debug trace
```
