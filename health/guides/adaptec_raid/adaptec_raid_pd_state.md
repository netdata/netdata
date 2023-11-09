### Understand the Alert

A RAID controller is a card or chip located between the operating system and a storage drive (usually a hard drive). This is an alert about the Adaptec raid controller. The Netdata Agent checks the physical device statuses which are managed by your raid controller.

This alert is triggered in critical state when the physical device is offline. 

### Troubleshoot the Alert

- Check the Offline Disk

Use the `arcconf` CLI tool to identify which drive or drives are offline:

```
root@netdata # arcconf GETCONFIG 1 AL
```

This command will display the configuration of all the managed Adaptec RAID controllers in your system. Check the "DEVICE #" and "DEVICE_DEFINITION" fields for details about the offline devices.

- Examine RAID Array Health

Check the array health status to better understand the overall array's stability and functionality:

```
root@netdata # arcconf GETSTATUS 1
```

This will provide an overview of your RAID controller's health status, including the operational mode, failure state, and rebuild progress (if applicable).

- Replace the Offline Disk

Before replacing an offline disk, ensure that you have a current backup of your data. Follow these steps to replace the drive:

1. Power off your system.
2. Remove the offline drive.
3. Insert the new drive.
4. Power on your system.

After the drive replacement, the Adaptec RAID card should automatically start rebuilding the faulty disk drive using the new disk. You can check the progress of the rebuild process with the `arcconf` command:

```
root@netdata # arcconf GETSTATUS 1
```

- Monitor Rebuild Progress

It's essential to monitor the RAID array's rebuild process to ensure it completes successfully. Use the `arcconf` command to verify the rebuild status:

```
root@netdata # arcconf GETSTATUS 1
```

This command will display the progress and status of the rebuild process. Keep an eye on it until it's completed.

- Verify RAID Array Health

After the rebuild is complete, use the `arcconf` command again to verify the health status of the RAID array:

```
root@netdata # arcconf GETSTATUS 1
```

Make sure that the RAID array's status is "Optimal" or "Ready" and that the replaced disk drive is now online.

### Useful Resources

1. [Adaptec Command Line Interface User’s Guide](https://download.adaptec.com/pdfs/user_guides/microsemi_cli_smarthba_smartraid_v3_00_23484_ug.pdf)
