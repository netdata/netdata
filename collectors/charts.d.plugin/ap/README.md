# Access Point Plugin (ap)

The `ap` collector visualizes data related to access points.

The source code is [here](https://github.com/netdata/netdata/blob/master/charts.d/ap.chart.sh).

## Example netdata charts

![image](https://cloud.githubusercontent.com/assets/2662304/12377654/9f566e88-bd2d-11e5-855a-e0ba96b8fd98.png)

## How it works

It does the following:

1. Runs `iw dev` searching for interfaces that have `type AP`.

   From the same output it collects the SSIDs each AP supports by looking for lines `ssid NAME`.

   Example:
```sh
# iw dev
phy#0
        Interface wlan0
                ifindex 3
                wdev 0x1
                addr 7c:dd:90:77:34:2a
                ssid TSAOUSIS
                type AP
                channel 7 (2442 MHz), width: 20 MHz, center1: 2442 MHz
```


2. For each interface found, it runs `iw INTERFACE station dump`.

   From the output is collects:

   - rx/tx bytes
   - rx/tx packets
   - tx retries
   - tx failed
   - signal strength
   - rx/tx bitrate
   - expected throughput

   Example:

```sh
# iw wlan0 station dump
Station 40:b8:37:5a:ed:5e (on wlan0)
        inactive time:  910 ms
        rx bytes:       15588897
        rx packets:     127772
        tx bytes:       52257763
        tx packets:     95802
        tx retries:     2162
        tx failed:      28
        signal:         -43 dBm
        signal avg:     -43 dBm
        tx bitrate:     65.0 MBit/s MCS 7
        rx bitrate:     1.0 MBit/s
        expected throughput:    32.125Mbps
        authorized:     yes
        authenticated:  yes
        preamble:       long
        WMM/WME:        yes
        MFP:            no
        TDLS peer:      no
```

3. For each interface found, it creates 6 charts:

   - Number of Connected clients
   - Bandwidth for all clients
   - Packets for all clients
   - Transmit Issues for all clients
   - Average Signal among all clients
   - Average Bitrate (including average expected throughput) among all clients

## Configuration

You can only set `ap_update_every=NUMBER` to `/etc/netdata/charts.d/ap.conf`, to give the data collection frequency.
To edit this file on your system run `/etc/netdata/edit-config charts.d/ap.conf`.

## Auto-detection

The plugin is able to auto-detect if you are running access points on your linux box.
