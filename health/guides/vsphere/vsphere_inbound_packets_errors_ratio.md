# vsphere_inbound_packets_errors_ratio

## Virtual Machine | Network

This alert presents the ratio of inbound packet errors for the network interface over the last 10
minutes.

The percentage of dropped packets is calculated over the last 10 minutes. To raise the alert, the minimum number of packets must be at least 10k within the last 10 minutes; otherwise the alert is never raised.

If the value is >= 2% the alarm gets raised into the warning state.

<details><summary>What are Packet Errors?</summary>

A packet error means there’s something wrong with the packet. There are two types of packet
errors that usually occur:
- Transmission errors, where a packet is damaged on its way to its destination – like a fragile Amazon order that gets dinged up en route.
- Format errors, where a packet’s format isn’t what the receiving device was expecting (or wanting). Think ordering a Coca-Cola in a restaurant and getting a Pepsi instead.

Packets can easily become damaged on their way through a network. Common reasons for damaged packages are if a device is connected to Ethernet through a:
- Bad cable 
- Bad port 
- Broken fiber cable
- Dirty fiber connector

Access points are also susceptible to packet errors. Offices often have multiple sources of
high radio frequency interference thanks to Bluetooth devices, unmanaged access points,
microwaves, and more. So packets traveling wirelessly are easily damaged.

If a packet error occurs, TCP (Transmission Control Protocol) will resend the same information
repeatedly, in hopes the data will eventually reach the destination without any problems.
UDP (User Datagram Protocol) will keep trucking forward even when packets fail to reach
their destination.<sup>[1](https://www.auvik.com/franklyit/blog/packet-errors-packet-discards-packet-loss/) </sup>
</details>

For further information, please have a look at the *References and Sources* section.

<details><summary>References and Sources</summary>

1. [Packet Errors](https://www.auvik.com/franklyit/blog/packet-errors-packet-discards-packet-loss/)

2. [VMware Documentation](
   https://docs.vmware.com/en/VMware-vSphere/7.0/com.vmware.vsphere.networking.doc/GUID-6DB73F20-C99A-43D4-9EE0-3277974EF8BF.html)
</details>

### Troubleshooting Section

To find out why the alert was raised, follow the steps in the [VMware Documentation](
https://docs.vmware.com/en/VMware-vSphere/7.0/com.vmware.vsphere.networking.doc/GUID-6DB73F20-C99A-43D4-9EE0-3277974EF8BF.html).
