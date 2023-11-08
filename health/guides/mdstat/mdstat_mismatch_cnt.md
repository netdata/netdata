### Understand the alert

This alert presents the number of unsynchronized blocks for the RAID array in crisis. Receiving this alert indicates a high number of unsynchronized blocks for the RAID array.  This might indicate that data on the array is corrupted.

This alert is raised to warning when the metric exceeds 1024 unsynchronized blocks.

### Troubleshoot the alert

There is no standard approach to troubleshooting this alert because the reasons can be various. 

For example, one of the reasons might be a swap on the array, which is relatively harmless. However, this alert can also be triggered by hardware issues which can lead to many problems and inconsistencies between the disks. 

### Useful resources

[Serverfault | Reasons for high mismatch_cnt on a RAID1/10 array](https://serverfault.com/questions/885565/what-are-raid-1-10-mismatch-cnt-0-causes-except-for-swap-file/885574#885574)
