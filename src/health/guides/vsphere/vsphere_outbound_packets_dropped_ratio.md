### Understand the alert

This alert calculates the ratio of `outbound dropped packets` for a network interface on a VMware vSphere Virtual Machine over the last 10 minutes. If you receive this alert, it means your Virtual Machine may be experiencing network performance issues due to dropped packets.

### What does outbound dropped packets mean?

Outbound dropped packets are network packets that are discarded by a network interface when they are supposed to be transmitted (sent) from the Virtual Machine to the destination. This can be caused by several factors, such as network congestion, insufficient buffer resources, or malfunctioning hardware.

### What can cause a high ratio of outbound dropped packets?

There are several possible reasons for a high ratio of outbound dropped packets, including:

1. Network congestion: High traffic may cause your network interface to drop packets if it cannot process all the outbound packets fast enough.
2. Insufficient buffer resources: The network interface requires buffer memory to store and process outbound packets. If not enough buffer memory is available, packets may be dropped.
3. Malfunctioning hardware: Issues with network hardware, such as the network adapter, could result in dropped packets.

### Troubleshoot the alert

- Check for network congestion
  1. Monitor your network traffic using monitoring tools such as `vSphere Client`, `vRealize Network Insight`, or other third-party tools.
  2. Identify whether there is an increase in traffic that could be causing congestion.
  3. Resolve any issues related to the cause of the increased traffic to relieve the congestion.

- Inspect buffer resources
  1. Use `vSphere Client` to check your Virtual Machine's network interface settings for correct buffer allocation.
  2. Increase buffer allocation if required or tune the buffer settings to ensure better resource usage.

- Verify network hardware
  1. Check the status of the network adapter using the `vSphere Client` or the VMware vSphere Command-Line Interface (vSphere CLI). Look for any signs of errors or issues.
  2. Verify that the network adapter driver is up-to-date and compatible with your vSphere environment.
  3. Consider troubleshooting or replacing the network adapter if hardware issues are suspected.

### Useful resources

1. [vSphere Monitoring and Performance Documentation (VMware Documentation)](https://docs.vmware.com/en/VMware-vSphere/7.0/com.vmware.vsphere.monitoring.doc/GUID-4D4F408E-F28E-4D34-A769-EEE9D9EB02AD.html)
2. [vSphere Administration Guide](https://docs.vmware.com/en/VMware-vSphere/index.html)
3. [vRealize Network Insight](https://www.vmware.com/products/vrealize-network-insight.html)