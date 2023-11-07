# mdstat_disks

## OS: Any

This alert presents the number of devices in the down state for the respective RAID array raising 
it.  
If you receive this alert, then the array is degraded and some array devices are missing.

- This alert is escalated to a warning when there are failed devices.

<details>
<summary>What is a "degraded array" event?</summary>

> When a RAID array experiences the failure of one or more disks, it can enter degraded mode, a
> fallback mode that generally allows the continued usage of the array, but either loses the
> performance boosts of the RAID technique (such as a RAID-1 mirror across two disks when one of
> them fails; performance will fall back to that of a normal, single drive) or experiences severe
> performance penalties due to the necessity to reconstruct the damaged data from error correction
> data.<sup>[1](https://en.wikipedia.org/wiki/Degraded_mode) </sup>

</details>

<br>

<details>
<summary>References and Sources</summary>

1. [Degraded Mode](https://en.wikipedia.org/wiki/Degraded_mode)
2. [Mdadm recover degraded array procedure](
   https://www.thomas-krenn.com/en/wiki/Mdadm_recover_degraded_Array_procedure)
3. [mdadm Manual page](https://linux.die.net/man/8/mdadm)
4. [mdadm cheat sheet](https://www.ducea.com/2009/03/08/mdadm-cheat-sheet/)

</details>


### Troubleshooting Section

<details>
<summary>Examine for faulty or offline devices</summary>

Having a degraded array means that one or more devices are faulty or missing.  
To fix this issue, check for faulty devices by running:

```
root@netdata~ # mdadm --detail <RAIDDEVICE>
```

Replace "RAIDDEVICE" with the name of your RAID device.

To recover the array, replace the faulty devices or bring back any offline 
devices.  
For more information check: [Mdadm recover degraded array procedure](
https://www.thomas-krenn.com/en/wiki/Mdadm_recover_degraded_Array_procedure)

</details>

