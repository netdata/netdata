# smartd_log

Module monitor `smartd` log files to collect HDD/SSD S.M.A.R.T attributes.

**Requirements:**
* `smartmontools`

It produces following charts for SCSI devices:

1. **Read Error Corrected**

2. **Read Error Uncorrected**

3. **Write Error Corrected**

4. **Write Error Uncorrected**

5. **Verify Error Corrected**

6. **Verify Error Uncorrected**

7. **Temperature**


For ATA devices:

1. **Soft Read Error Rate**

2. **SATA Interface Downshift**

3. **UDMA CRC Error Count**

4. **Throughput Performance**

5. **Seek Time Performance**

6. **Start/Stop Count**

7. **Power-On Hours Count**

8. **Power Cycle Count**

9. **Unexpected Power Loss**

10. **Spin-Up Time**

11. **Spin-up Retries**

12. **Calibration Retries**

13. **Temperature**

14. **Reallocated Sectors Count**

15. **Reserved Block Count**

16. **Program Fail Count**

17. **Erase Fail Count**

18. **Wear Leveller Worst Case Erase Count**

19. **Unused Reserved NAND Blocks**

20. **Reallocation Event Count**

21. **Current Pending Sector Count**

22. **Offline Uncorrectable Sector Count**

23. **Percent Lifetime Used**

### prerequisite
`smartd` must be running with `-A` option to write smartd attribute information to files.

For this you need to set `smartd_opts` (or `SMARTD_ARGS`, check _smartd.service_ content) in `/etc/default/smartmontools`:


```
# dump smartd attrs info every 600 seconds
smartd_opts="-A /var/log/smartd/ -i 600"
```


`smartd` appends logs at every run. It's strongly recommended to use `logrotate` for smartd files.

### configuration

```yaml
local:
  log_path : '/var/log/smartd/'
```

If no configuration is given, module will attempt to read log files in `/var/log/smartd/` directory.

---
