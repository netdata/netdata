### Understand the alert

This alert is related to the Adaptec RAID controller, which manages the logical device statuses on your RAID configuration. When this alert is triggered in a critical state, it means that a logical device state value is in a degraded or failed state, indicating that one or more disks in your RAID configuration have failed.

### Troubleshoot the alert

Data is priceless. Before taking any action, ensure to have necessary backups in place. Netdata is not liable for any loss or corruption of any data, database, or software.

Your Adaptec RAID card will automatically start rebuilding a faulty hard drive when you replace it with a healthy one. Sometimes this operation may take some time or may not start automatically.

#### 1. Verify that a rebuild is not in process

Check if the rebuild process is already running:

```
root@netdata # arcconf GETSTATUS <Controller_num>
```

Replace `<Controller_num>` with the number of your RAID controller.

#### 2. Check for idle/missing segments of logical devices

Examine the output of the previous command to find any segments that are idle or missing.

#### 3. Manually change your ld status

If the rebuild process hasn't started automatically, change the logical device (ld) status manually. This action will trigger a rebuild on your RAID:

```
root@netdata # arcconf SETSTATE <Controller_num> LOGICALDRIVE <LD_num> OPTIMAL ADVANCED nocheck noprompt
```

Replace `<Controller_num>` with the number of your RAID controller and `<LD_num>` with the number of the logical device.

### Useful resources

1. [Microsemi Adaptec ARCCONF User's Guide](https://download.adaptec.com/pdfs/user_guides/microsemi_cli_smarthba_smartraid_v3_00_23484_ug.pdf)
