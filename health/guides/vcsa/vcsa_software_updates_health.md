# vcsa_software_updates_health

## Virtual Machine | VMware vCenter

This alert presents the software updates availability status.  
The values can be:

| Code |                              Color                              | Description                                          | Alert Status |
|:----:|:---------------------------------------------------------------:|:-----------------------------------------------------|:------------:|
| `-1` |                            no color                             | Unknown.                                             |    Clear     |
| `0`  | ![#00FF00](https://via.placeholder.com/18/00FF00/000000?text=+) | no updates available.                                |    Clear     |
| `2`  | ![#ffa500](https://via.placeholder.com/18/ffa500/000000?text=+) | non-security updates are available.                  |    Clear     |
| `3`  | ![#f03c15](https://via.placeholder.com/18/f03c15/000000?text=+) | security updates are available.                      |   Critical   |
| `4`  | ![#808080](https://via.placeholder.com/18/808080/000000?text=+) | an error retrieving information on software updates. |   Warning    |

For further information, please have a look at the *References and Sources* section.

<details><summary>References and Sources</summary>

1. [VMware Documentation](https://docs.vmware.com/en/VMware-vSphere/7.0/com.vmware.vsphere.vcenter.configuration.doc/GUID-52AF3379-8D78-437F-96EF-25D1A1100BEE.html)

</details>


### Troubleshooting Section

If the alert was raised into critical, proceed by installing the security updates that are 
available. If the alert was raised into warning, consider viewing the details in the Health Messages 
pane.

You can also find more details in the [VMware vCenter Server documentation](https://docs.vmware.com/en/VMware-vSphere/7.0/com.vmware.vsphere.vcenter.configuration.doc/GUID-52AF3379-8D78-437F-96EF-25D1A1100BEE.html).
