# networkviewer.plugin

This plugin uses eBPF to bring connection information from the kernel ring to user ring. 

The default behavior of this plugin is to monitor all connections from the privates IPs defined inside the RFC 1918 
(https://tools.ietf.org/html/rfc1918) to any destination, this default behavior can be changed with the configuration file
`networkviewer.conf` .

## Only Linux for while

This plugin for while works only on Linux, but we are monitoring the development of eBPF on other Operate Systems.

## Charts

Network viewer, for while monitors all IPV4 connections made with TCP and UDP, it displays information in the following
charts:

- Transfer inbound: Displays the number of bytes received as response of the previous request.
- Transfer outbound: The number of bytes sent to specific ports
- Active connections: Number of connections made to a specific service.

## Performance

The number of connections that network viewer monitors was always a great concern for Netdata, during our development
we notice that our code in average uses 1% of the CPU a result compatible with TCPdump software.

## Configuration

The configuration file for the plugin is `/etc/netdata/network_viewer.conf`, inside this file you have the following 
options:

### Outbound

This option sets the source addresses monitored inside the plugin, the default values for this option is

```
outbound = 10.0.0.0/8 172.16.0.0-255.240.0.0 192.168.0.0/255.255.0.0
```

As you can see in the previous values, netdata accepts different range format for `outbound` and the next option.

### Inbound

This option sets the destination options to monitor, the default value is the whole internet

```
inbound = 0.0.0.0/0
```

### Destination Ports

Netdata is capable to monitor all the 65536 ports, but this will slow down the chart rendering, to avoid this problem 
Netdata selected 50 most common ports as default ports to monitor.

```
destination_ports = 20,21,22,23, 25,43,53,80,88,110,118,123,135, 137 138 139 143, 156, 194, 389, 443, 445, 464, 465, 513, 520, 530, 546, 547, 563, 631, 636, 691, 749, 901, 989, 990, 993, 995, 1381, 1433 1434 1512 1525 3389 3306 5432 6000 8080 19999
```

we also allow different separators (either space or comma) for the destination ports.

### Dimension name

Netdata calls functions available on Linux to name the dimensions, so instead to see a dimension with the value `80` 
you will have a dimension `http`, but case you wanna use names that are simplest to you understand the charts, you can
set them using the format `port = name`, by default Netdata sets its default port name as example

```
19999 = Netdata
```
