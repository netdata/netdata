# ioping_disk_latency

## OS: Any

This alarm presents the average `I/O latency` over the last 10 seconds.

> `I/O latency` is the time that is required to complete a single I/O operation on a block device.

If this alarm is raised, it might indicate that your disk is under high load, or that the disk is slow.

### Troubleshooting Section

<details>
<summary>Check related charts to find your case</summary>

- First, you need to identify whether your disk is under high load or not.

    <br>

    1. Go to your node on the Netdata Cloud, on the `Disks` section, and select the disk you want to
       investigate.

    <br>

    2. On the top of the page you can see the utilization of the disk, and you can also go to the `Disk I/O Bandwidth`
       chart. There you can see the amount of transferred data `to` and `from` the particular disk.

    <br>

- If the utilization is low and there isn't a significant amount of data being transferred, your
  drive's latency is slow, so you need to upgrade that drive. (If it is an HDD, then an SSD would be a significant upgrade
  for latency issues.)


- If there is a high load on the drive, it usually means that one or more processes are heavily utilizing the drive.
</details>