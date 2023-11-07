# megacli_pd_predictive_failures

## OS: Any

A disk array controller is a device that manages the physical disk drives and presents them to the
computer as logical units. It almost always implements hardware RAID, thus it is sometimes referred
to as RAID controller. It also often provides additional disk cache.

A predictive drive failure (self-monitoring analysis and reporting
technology [S.M.A.R.T.](https://en.wikipedia.org/wiki/S.M.A.R.T.#:~:text=(Self%2DMonitoring%2C%20Analysis%20and,SSDs)%2C%20and%20eMMC%20drives)
error).

This is an alert about the physical disks attached to the MegaCLI controller. The Netdata Agent
calculates the number of physical drive predictive failures. The failure prediction function for the
hard disk drives determines the risk of a failure in advance and issues a warning when the risk is
high. A hard disk can still operate normally but may fail in the near future. You might want to
consider replacing the disk.

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


3. Consult the manual's <sup>[1](https://docs.broadcom.com/docs/12353236) </sup>
    1. section `2.1.16` to check what is going wrong with your drives.
    2. section `7.18` to perform any action in drives. Focus on {`7.18.2`,`7.18.6`,`7.18.7`,`7.18.8`
       ,`7.18.11`,`7.18.14`}
