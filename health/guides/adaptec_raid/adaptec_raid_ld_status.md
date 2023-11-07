# adaptec_raid_ld_status

## OS: Any

A RAID controller is a card or chip located between the operating system and a storage drive (usually 
a hard drive). This is an alert about the Adaptec raid controller. The Netdata Agent checks
the logical device statuses which are managed by your raid controller.

This alert is triggered in critical state when a logical device state value is in degraded or failed
state. This can indicate that one or more disks in your RAID configuration failed. Below you can
find how tolerant is each raid configuration in cases of disk failures.

<details>
<summary>Fault tolerance for the most popular raid configurations </summary>

- _RAID 0_ provides no fault tolerance. Any drive failures will cause data loss, so do not use this
  on a mission critical server.

- _RAID 1_ configuration is best used for situations where capacity isn't a requirement but data
  protection is. This set up mirrors two disks so you can have 1 drive fail and still be able to
  recover your data.

- _RAID 5_  can withstand a single drive failure with a tradeoff in performance.

- _RAID 6_ can withstand two disk failures at one time.

- _RAID 10_  can survive a single drive failure per array.

</details>

Your system manages your Adaptec raid controller via the ARCCONF command line tool. You can find
more information about this utility from
the [user's guide for the ARCCONF](https://download.adaptec.com/pdfs/user_guides/microsemi_cli_smarthba_smartraid_v3_00_23484_ug.pdf).

### Troubleshooting section

Data is priceless. Before you perform any action, make sure that you have taken any necessary backup
steps. Netdata is not liable for any loss or corruption of any data, database, or software.

Your Adaptec RAID card will automatically start to rebuild a faulty hard drive when you
replace it with a healthy one. Sometimes this operation may take some time or may not start
automatically.


<details>
<summary>Manually change the status of your ld </summary>
This action will trigger a rebuild on your RAID.

1. Verify that a rebuild is not in process.
  
    ```
    root@netdata # arcconf GETSTATUS <Controller_num>
    ```
   
    2. Check for idle/missing segments of logical devices.



3. Manually change your ld status

    ```
    root@netdata # arcconf SETSTATE <Controller_num> LOGICALDRIVE <LD_num> OPTIMAL ADVANCED nocheck noprompt
    ```

</details>
