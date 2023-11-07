# vcsa_system_health

## Virtual Machine | VMware vCenter

This alert presents the overall system health status.
It can take the values:

- -1: unknown (no color)
- 0: all components are healthy. (green)
- 1: one or more components might become overloaded soon. (yellow)
- 2: one or more components in the appliance might be degraded. (orange)
- 3: one or more components might be in an unusable status and the appliance might become unresponsive soon. (red)
- 4: no health data is available. (grey)


If you receive this alert, it means that the overall system status is unhealthy. One or more 
components might become overloaded soon (yellow), or might be degraded (orange), or 
might be in an unusable status and the appliance might become unresponsive soon (red).  

This alert is raised into warning if the status has a code of 1 or 2.  
If the metric reaches a value of 3, the alert is raised into critical.

For further information, please have a look at the *References and Sources* section.

<details><summary>References and Sources</summary>

1. [VMware Documentation](https://docs.vmware.com/en/VMware-vSphere/7.0/com.vmware.vsphere.vcenter.configuration.doc/GUID-52AF3379-8D78-437F-96EF-25D1A1100BEE.html)

</details>


### Troubleshooting Section

To troubleshoot the issue, you need to log into vCenter Server Management Interface and follow the information in the [vmware documentation](https://docs.vmware.com/en/VMware-vSphere/7.0/com.vmware.vsphere.vcenter.configuration.doc/GUID-52AF3379-8D78-437F-96EF-25D1A1100BEE.html).
