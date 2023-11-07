# adaptec_raid_pd_state

## OS: Any

A RAID controller is a card or chip located between the operating system and a storage drive (usually a hard drive). This is an alert about the Adaptec raid controller. The Netdata Agent checks
the physical device statuses which are managed by your raid controller.

This alert is triggered in critical state when the physical device is offline. Below you can
find how tolerant each RAID configuration is in cases of disk failures.

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
more information about this utility in
the [user's guide for the ARCCONF](https://download.adaptec.com/pdfs/user_guides/microsemi_cli_smarthba_smartraid_v3_00_23484_ug.pdf).

### Troubleshooting section

Data is priceless. Before you perform any action, make sure that you have taken any necessary backup
steps. Netdata is not liable for any loss or corruption of any data, database, or software.


<details>
<summary>Verify a bad disk </summary>

Check the smart report for the drives in your RAID controller:

    ```
    root@netdata # arcconf GETSMARTSTATS 1
    ```

If a disk is degraded, you should consider replacing it. Your Adaptec RAID card will
   automatically start to rebuild a faulty hard drive when you replace it with a healthy one.

