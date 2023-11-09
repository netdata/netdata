### Understand the alert

This alert indicates the current speed of the network interface `${label:device}`. If you receive this alert, it means that there is a significant change or reduction in the speed of your network interface.

### What does interface speed mean?

Interface speed refers to the maximum throughput an interface (network card or adapter) can support in terms of transmitting and receiving data. It is measured in Megabits per second (Mbit/s) and determines the performance of a network connection.

### Troubleshoot the alert

- Check the network interface speed.

To see the interface speed and other information about the network interface, run the following command in the terminal:

```
ethtool ${label:device}
```

Replace `${label:device}` with your network interface name, e.g., `eth0` or `enp2s0`.

- Confirm if there is a network congestion issue.

High network traffic or congestion might cause reduced interface speed. Use the `iftop` utility to monitor the traffic on the network interface. If you don't have `iftop` installed, then [install it](https://www.binarytides.com/linux-commands-monitor-network/).

Run the following command in the terminal:

```
sudo iftop -i ${label:device}
```

Replace `${label:device}` with your network interface name.

- Verify cable connections and quality.

Physical cable issues might cause reduced speed in the network interface. Check the connections and quality of the cables connecting your system to the network devices such as routers, switches, or hubs.

- Update network drivers.

Outdated network drivers can also lead to reduced speed in the network interface. Update the network drivers to the latest version to avoid any compatibility issues or performance degradations.

- Check for EMI (Electromagnetic Interference).

Network cables and devices located near power cables or electronic devices producing electromagnetic fields might experience reduced network interface speed. Make sure that your network cables and devices are not in proximity to potential sources of EMI.

