<!--
title: "Nvidia GPU monitoring with Netdata"
custom_edit_url: "https://github.com/netdata/netdata/edit/master/collectors/python.d.plugin/nvidia_smi/README.md"
sidebar_label: "nvidia_smi-python.d.plugin"
learn_status: "Published"
learn_topic_type: "References"
learn_rel_path: "Integrations/Monitor/Devices"
-->

# Nvidia GPU collector

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

## Docker

GPU monitoring in a docker container is possible with [nvidia-container-toolkit](https://docs.nvidia.com/datacenter/cloud-native/container-toolkit/install-guide.html) installed on the host system, and `gcompat` added to the `NETDATA_EXTRA_APK_PACKAGES` environment variable.

Sample `docker-compose.yml`
```yaml
version: '3'
services:
  netdata:
    image: netdata/netdata
    container_name: netdata
    hostname: example.com # set to fqdn of host
    ports:
      - 19999:19999
    restart: unless-stopped
    cap_add:
      - SYS_PTRACE
    security_opt:
      - apparmor:unconfined
    environment:
      - NETDATA_EXTRA_APK_PACKAGES=gcompat
    volumes:
      - netdataconfig:/etc/netdata
      - netdatalib:/var/lib/netdata
      - netdatacache:/var/cache/netdata
      - /etc/passwd:/host/etc/passwd:ro
      - /etc/group:/host/etc/group:ro
      - /proc:/host/proc:ro
      - /sys:/host/sys:ro
      - /etc/os-release:/host/etc/os-release:ro
    deploy:
      resources:
        reservations:
          devices:
          - driver: nvidia
            count: all
            capabilities: [gpu]

volumes:
  netdataconfig:
  netdatalib:
  netdatacache:
```

Sample `docker run`
```yaml
docker run -d --name=netdata \
  -p 19999:19999 \
  -e NETDATA_EXTRA_APK_PACKAGES=gcompat \
  -v netdataconfig:/etc/netdata \
  -v netdatalib:/var/lib/netdata \
  -v netdatacache:/var/cache/netdata \
  -v /etc/passwd:/host/etc/passwd:ro \
  -v /etc/group:/host/etc/group:ro \
  -v /proc:/host/proc:ro \
  -v /sys:/host/sys:ro \
  -v /etc/os-release:/host/etc/os-release:ro \
  --restart unless-stopped \
  --cap-add SYS_PTRACE \
  --security-opt apparmor=unconfined \
  --gpus all \
  netdata/netdata
```

### Docker Troubleshooting
To troubleshoot `nvidia-smi` in a docker container, first confirm that `nvidia-smi` is working on the host system. If that is working correctly, run `docker exec -it netdata nvidia-smi` to confirm it's working within the docker container. If `nvidia-smi` is fuctioning both inside and outside of the container, confirm that `nvidia-smi: yes` is uncommented in `python.d.conf`.
```bash
docker exec -it netdata bash
cd /etc/netdata
./edit-config python.d.conf
```
