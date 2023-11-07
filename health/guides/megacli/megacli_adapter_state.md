# megacli_adapter_state

## OS: Any

A disk array controller is a device that manages the physical disk drives and presents them to the
computer as logical units. It almost always implements hardware RAID, thus it is sometimes referred
to as RAID controller. It also often provides additional disk cache.

The Netdata Agent checks the status of your MegaRAID controller by scraping the output of
the `megacli -LDPDInfo -aAll` command. This alert indicates that the status of a virtual drive is in
the degraded state (0: false, 1:true).

#### States of a virtual drive:

|      State       | Description                                                                                                                                                                    |      
|:----------------:|:-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
|     Optimal      | The virtual drive operating condition is good. All configured drives are online.                                                                                               | 
|     Degraded     | The virtual drive operating condition is not optimal. One of the configured drives has failed or is offline.                                                                   | 
| Partial Degraded | The operating condition in a RAID 6 virtual drive is not optimal. One of the configured drives has failed or is offline. RAID 6 can tolerate up to two drive failures.         | 
|      Failed      | The virtual drive has failed.                                                                                                                                                  | 
|     Offline      | The virtual drive is not available to the RAID controller.                                                                                                                     | 

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

2. Check which virtual drive is in degraded state and in which adapter

3. Consult the manual's <sup>[1](https://docs.broadcom.com/docs/12353236) </sup>
    1. section `2.1.16` to check what is going wrong with your drives.
    2. section `7.18` to perform any action in drives. Focus on {`7.18.2`,`7.18.6`,`7.18.7`,`7.18.8`
       ,`7.18.11`,`7.18.14`}

</details>