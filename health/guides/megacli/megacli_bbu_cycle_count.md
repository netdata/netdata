### Understand the alert

The `megacli_bbu_cycle_count` alert is related to the battery backup unit (BBU) of your MegaCLI controller. This alert is triggered when the average number of full recharge cycles during the BBU's lifetime exceeds a predefined threshold. High numbers of charge cycles can affect the battery's relative capacity.

A warning state is triggered when the number of charge cycles is greater than 100, and a critical state is triggered when the number of charge cycles is greater than 500.

### Troubleshoot the alert

**Caution:** Before performing any troubleshooting steps, ensure that you have taken the necessary backup measures to protect your data. Netdata is not liable for any data loss or corruption.

1. Gather information about the battery units for all of your adapters:

   ```
   megacli -AdpBbuCmd -GetBbuStatus -aALL
   ```

2. Perform a battery check on the BBU with a low relative charge. Before taking any action, consult the manual's[section 7.14](https://docs.broadcom.com/docs/12353236):

   ```
   megacli -AdpBbuCmd -BbuLearn -aX // X is the adapter's number
   ```

3. If necessary, replace the battery in question.

### Useful resources

1. [MegaRAID SAS Software User Guide (PDF download)](https://docs.broadcom.com/docs/12353236)
2. [MegaCLI commands cheatsheet](https://www.broadcom.com/support/knowledgebase/1211161496959/megacli-commands)