### Understand the alert

This alert indicates that the status of a virtual drive on your MegaRAID controller is in a degraded state. A degraded state means that the virtual drive's operating condition is not optimal, and one of the configured drives has failed or is offline.

### Troubleshoot the alert

#### General approach

1. Gather more information about your virtual drives in all adapters:

```
root@netdata # megacli –LDInfo -Lall -aALL
```

2. Check which virtual drive is in a degraded state and in which adapter.

3. Consult the MegaRAID SAS Software User Guide [1]:

   1. Section `2.1.16` to check what is going wrong with your drives.
   2. Section `7.18` to perform any action on drives. Focus on sections `7.18.2`, `7.18.6`, `7.18.7`, `7.18.8`, `7.18.11`, and `7.18.14`.

### Warning

Data is priceless. Before performing any action, make sure that you have taken any necessary backup steps. Netdata is not liable for any loss or corruption of any data, database, or software.

### Useful resources

1. [MegaRAID SAS Software User Guide [PDF download]](https://docs.broadcom.com/docs/12353236)
2. [MegaCLI commands cheatsheet](https://www.broadcom.com/support/knowledgebase/1211161496959/megacli-commands)