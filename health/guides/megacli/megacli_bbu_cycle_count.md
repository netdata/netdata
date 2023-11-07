# megacli_bbu_cycle_count

## OS: Any

This is an alert about the battery backup unit in the MegaCLI controller. The Netdata Agent monitors
the average battery backup unit charge cycles count over the last 10 seconds. This alert indicates
that a high number of full recharge cycles have been elapsed in the unit's lifetime. This metrics
may affect the battery relative capacity.

This alert is triggered in warning state when the number of charge cycles is greater than 100 and in
critical state when it is greater than 500.

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

