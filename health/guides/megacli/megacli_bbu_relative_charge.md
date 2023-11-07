# megacli_bbu_relative_charge

## OS: Any

A disk array controller is a device that manages the physical disk drives and presents them to the
computer as logical units. It almost always implements hardware RAID, thus it is sometimes referred
to as RAID controller. It also often provides additional disk cache.

The Netdata Agent calculates the average battery backup unit relative state of charge over the last
10 seconds. This alert indicates that the state of charge is low. The relative state of charge is an
indication of full charge capacity percentage in relation to the design capacity. A constantly low
value may indicate that the battery is worn out. You might want to consider changing the battery.

This alert is raised into warning when the relative state of charge of a battery is below 80% and in
critical when it is below 50%.


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

1. Gather more information about your battery units in all of your adapters

      ```
      root@netdata # megacli -AdpBbuCmd -GetBbuStatus -aALL
      ```

2. Perform a battery check in the battery which had low relative charge. **Before perform any
   action, consult the manual's <sup>[1](https://docs.broadcom.com/docs/12353236) </sup>
   section {`7.14`}**

      ```
      root@netdata # megacli -AdpBbuCmd -BbuLearn -aX // X is the adaptor's number
      ```

3. Replace the battery in question if needed.

</details>
