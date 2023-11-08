### Understand the alert

This alert indicates that the number of leased IP addresses in your DHCP range, managed by dnsmasq, is close to the total number of provisioned DHCP addresses. The alert will be triggered in a warning state when the percentage of leased IP addresses is between 80-90% and in a critical state when it is between 90-95%.

### What is DHCP?

Dynamic Host Configuration Protocol (DHCP) is a network management protocol that dynamically assigns IP addresses and other configuration information to devices connected to the network. It helps network administrators to manage the IP address allocation process efficiently.

### What is dnsmasq?

`dnsmasq` is a lightweight, easy to configure DNS forwarder, DHCP server, and TFTP server. It is designed to provide DNS and optionally, DHCP, services to a small-scale network. Dnsmasq can serve the names of local machines which are not in the global DNS.

### Troubleshoot the alert

1. Check the current DHCP lease utilization

To see the current percentage of DHCP leases in use, run the following command:

```
cat /var/lib/misc/dnsmasq.leases | wc -l
```

2. Verify the configured DHCP range

Check the `/etc/dnsmasq.conf` file to ensure that the DHCP range is configured correctly:

```
grep -i "dhcp-range" /etc/dnsmasq.conf
```

Make sure that the range provides enough IP addresses for the number of devices in your network.

3. Increase the DHCP range

If required, increase the number of available IP addresses within the DHCP range by modifying the `/etc/dnsmasq.conf` file, expanding the range and/or decreasing the lease time.

After modifying the configuration, restart the dnsmasq service to apply the changes:

```
sudo systemctl restart dnsmasq
```

4. Monitor the DHCP lease utilization

Keep monitoring the DHCP lease utilization to ensure that the new range and lease settings are sufficient for your network's needs.

### Useful resources

1. [The Dnsmasq Homepage](http://www.thekelleys.org.uk/dnsmasq/doc.html)
2. [Ubuntu Community Help Wiki: Dnsmasq](https://help.ubuntu.com/community/Dnsmasq)
