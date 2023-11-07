# megacli_pd_media_errors

## OS: Any

A disk array controller is a device that manages the physical disk drives and presents them to the
computer as logical units. It almost always implements hardware RAID, thus it is sometimes referred
to as RAID controller. It also often provides additional disk cache.

A media error is an event where a storage disk was unable to perform the requested I/O operation
because of problems accessing the stored data.

This is an alert about the physical disks attached to the MegaCLI controller. The Netdata Agent
monitors the number of physical drive media errors. This alert indicates that a bad sector was found
on the drive during a patrol check or from a rebuild operation on a specific disk by the raid
adapter.

This alert is raised into warning if any media error occur. This doesn't mean that there is an
imminent disk failure, but you should keep an eye on this particular disk

<details>
<summary> More about media errors </summary>

Media errors are more common on read transactions but might occur on writes as well. A media error
on a `write` may occur when the disk has problems locating the position to write the data. On reads,
in addition to these positioning faults, the disk may experience problems retrieving the data. When
a disk writes data, it writes other information as well, such as to record the position, note CRC or
checksum to confirm data write integrity.

</details>

<details>
<summary>References and source</summary>

1. [MegaRAID SAS Software User Guide \[pdf download\]](https://docs.broadcom.com/docs/12353236)
2. [MegaCLI commands cheatsheet](https://www.broadcom.com/support/knowledgebase/1211161496959/megacli-commands)

</details>

### Troubleshooting section:

Data is priceless. Before you perform any action, make sure that you have taken any necessary backup
steps. Netdata is not liable for any loss or corruption of any data, database, or software.

<details>
 <summary>General approach</summary>

1. Gather more information about your virtual drives in all adapters

      ```
      root@netdata # megacli –LDInfo -Lall -aALL
      ```

2. Check which virtual drive is reporting media errors and in which adapter

3. Check the Bad block table for the virtual drive in question

      ```
      root@netdata # megacli –GetBbtEntries -LX -aY  // X: virtual drive , Y the adapter
      ```

4. Consult the manual's <sup>[1](https://docs.broadcom.com/docs/12353236) </sup>
   section `7.17.11` to recheck these block entries. **This operation removes any data stored on the
   physical drives. Back up the good data on the drives before making any changes to the
   configuration**