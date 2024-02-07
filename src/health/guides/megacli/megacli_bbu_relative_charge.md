### Understand the alert

This alert is related to the disk array controller's battery backup unit (BBU) relative state of charge. If you receive this alert, it means that the battery backup unit's charge is low, which may affect your RAID controller's performance or lead to data loss in case of a power failure.

### What does low BBU relative charge mean?

A low BBU relative charge indicates that the state of charge is low compared to its design capacity. The relative state of charge is a percentage indication of the full charge capacity compared to its designed capacity. If the relative charge is constantly low, it may suggest that the battery is worn out and needs replacement.

### Troubleshoot the alert

1. Gather information about your battery units for all controllers:

   ```
   sudo megacli -AdpBbuCmd -GetBbuStatus -aALL
   ```

   This command will provide you with detailed information about the BBU status for each controller.

2. Perform a manual battery calibration (learning cycle) on the battery with a low relative charge:

   ```
   sudo megacli -AdpBbuCmd -BbuLearn -aX
   ```

   Replace `X` with the controller's number. Please consult the [MegaRAID SAS Software User Guide](https://docs.broadcom.com/docs/12353236), section 7.14, before performing this action.

   A learning cycle discharges and recharges the battery, which can help recalibrate the battery and improve its relative state of charge. However, it may temporarily disable the write cache during this process.

3. Monitor the BBU relative charge after the learning cycle. If the relative charge remains low, consider replacing the battery in question. Consult your hardware vendor's documentation for guidance on replacing the BBU.

### Useful resources

1. [MegaRAID SAS Software User Guide [pdf download]](https://docs.broadcom.com/docs/12353236)
2. [MegaCLI commands cheatsheet](https://www.broadcom.com/support/knowledgebase/1211161496959/megacli-commands)

**Note**: Data is priceless. Before you perform any action, make sure that you have taken any necessary backup steps. Netdata is not liable for any loss or corruption of any data, database, or software.