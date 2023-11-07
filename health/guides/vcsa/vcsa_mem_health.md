# vcsa_mem_health

## Virtual Machine | VMware vCenter

This alert presents the memory health status.  
The values for every component's health can be:

| Code |                              Color                              | Description                                                 | Alert Status |
|:----:|:---------------------------------------------------------------:|:------------------------------------------------------------|:------------:|
| `-1` |                            no color                             | Unknown.                                                    |    Clear     |
| `0`  | ![#00FF00](https://via.placeholder.com/18/00FF00/000000?text=+) | The component is healthy.                                   |    Clear     |
| `1`  | ![#ffea00](https://via.placeholder.com/18/ffea00/000000?text=+) | The component is healthy but may have some problems.        |   Warning    |
| `2`  | ![#ffa500](https://via.placeholder.com/18/ffa500/000000?text=+) | The component is degraded, and may have serious problems.   |   Critical   |
| `3`  | ![#f03c15](https://via.placeholder.com/18/f03c15/000000?text=+) | The component is unavailable or will stop functioning soon. |   Critical   |
| `4`  | ![#808080](https://via.placeholder.com/18/808080/000000?text=+) | No health data is available.                                |    Clear     |

For further information, please have a look at the *References and Sources* section.

<details><summary>References and Sources</summary>

1. [VMware Documentation](https://docs.vmware.com/en/VMware-vSphere/7.0/com.vmware.vsphere.vcenter.configuration.doc/GUID-ACEC0944-EFA7-482B-84DF-6A084C0868B3.html)

</details>

### Troubleshooting Section

To find out why the alert was raised, follow the steps in
the [vmware vCenter Server documentation](https://docs.vmware.com/en/VMware-vSphere/7.0/com.vmware.vsphere.vcenter.configuration.doc/GUID-ACEC0944-EFA7-482B-84DF-6A084C0868B3.html)
.