### Understand the alert

This alert is triggered when there are `packet drops` within the last 10 minutes in your system's `Quality of Service` (`QoS`). If you receive this alert, it means your system's `network performance` may be suffering due to dropped packets.

### What does packet drops mean?

Packet drops refer to situations where one or more packets of data traveling across a computer network fail to reach their destination, often caused by network congestion or faulty hardware. Dropped packets can result in poor QoS, including degraded voice and video quality, or even data loss in severe cases.

### Troubleshoot the alert

- Check the network utilization, packet loss, and latency

  You can use the `netdata` dashboard to check the network utilization, packet loss, and latency. This will help you identify if there is any congestion or excessive usage in your network that could be causing the packet drops.

- Examine the system logs

  Inspect your system logs to identify any potential hardware issues or network-related errors that could be causing the packet drops. You can use tools like `dmesg`, `journalctl`, or check the `/var/log` directory for log files.

- Check for faulty hardware or misconfigurations

  Inspect your network devices, such as routers, switches, and network interfaces, for any signs of faulty hardware or misconfigurations that could be causing dropped packets.

- Optimize your network configuration

  Review your network configuration for any settings that could be causing dropped packets, such as improper buffer sizes, incorrect QoS settings, or misconfigured packet handling mechanisms.

- Update network device drivers or firmware

  Ensure that you are using the latest drivers and firmware for your network devices. Outdated or buggy drivers can sometimes cause packet drops.

- Monitor the network continuously

  Regularly monitor the performance of your network to identify and address any issues that may be causing packet drops. You can use tools like `tc`, `ip`, `ifconfig`, and others for this purpose.

### Useful resources

1. [Netdata - Real-Time Performance Monitoring](https://www.netdata.cloud/)
2. [Linux Advanced Routing & Traffic Control](https://lartc.org/)
