### Understand the alert

The `megacli_pd_media_errors` alert is triggered when there are media errors on the physical disks attached to the MegaCLI controller. A media error is an event where a storage disk was unable to perform the requested I/O operation due to problems accessing the stored data. This alert indicates that a bad sector was found on the drive during a patrol check or from a rebuild operation on a specific disk by the RAID adapter. Although this does not mean imminent disk failure, it is a warning, and you should monitor the affected disk.

### Troubleshoot the alert

**Data is priceless. Before you perform any action, make sure that you have taken any necessary backup steps. Netdata is not liable for any loss or corruption of any data, database, or software.**

1. Gather more information about your virtual drives on all adapters:

   ```
   megacli –LDInfo -Lall -aALL
   ```

2. Check which virtual drive is reporting media errors and in which adapter.

3. Check the Bad block table for the virtual drive in question:

   ```
   megacli –GetBbtEntries -LX -aY  // X: virtual drive, Y: the adapter
   ```

4. Consult the MegaRAID SAS Software User Guide's section 7.17.11[^1] to recheck these block entries. **This operation removes any data stored on the physical drives. Back up the good data on the drives before making any changes to the configuration.**

### Useful resources

1. [MegaRAID SAS Software User Guide [PDF download]](https://docs.broadcom.com/docs/12353236)
2. [MegaCLI command cheatsheet](https://www.broadcom.com/support/knowledgebase/1211161496959/megacli-commands)

[^1]: https://docs.broadcom.com/docs/12353236