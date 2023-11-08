### Understand the alert

This alert indicates that one or more physical disks attached to the MegaCLI controller are experiencing predictive failures. A predictive failure is a warning that a hard disk may fail in the near future, even if it's still working normally. The failure prediction relies on the self-monitoring and analysis technology (S.M.A.R.T.) built into the disk drive.

### Troubleshoot the alert

**Make sure you have taken necessary backup steps before performing any action. Netdata is not liable for any loss or corruption of data, databases, or software.**

1. Identify the problematic drives:

   Use the following command to gather information about your virtual drives in all adapters:

   ```
   megacli –LDInfo -Lall -aALL
   ```

2. Determine the virtual drive and adapter reporting media errors.

3. Consult the MegaRAID SAS Software User Guide [1]:

   1. Refer to Section 2.1.16 to check for issues with your drives.
   2. Refer to Section 7.18 to perform any appropriate actions on drives. Focus on Sections 7.18.2, 7.18.6, 7.18.7, 7.18.8, 7.18.11, and 7.18.14.

4. Consider replacing the problematic disk(s) to prevent imminent failures and potential data loss.

### Useful resources

1. [MegaRAID SAS Software User Guide (PDF download)](https://docs.broadcom.com/docs/12353236)
2. [MegaCLI commands cheatsheet](https://www.broadcom.com/support/knowledgebase/1211161496959/megacli-commands)