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
1. **Read Error Rate**

2. **Seek Error Rate**

3. **Soft Read Error Rate**

4. **Write Error Rate**

5. **SATA Interface Downshift**

6. **UDMA CRC Error Count**

7. **Throughput Performance**

8. **Seek Time Performance**

9. **Start/Stop Count**

10. **Power-On Hours Count**

11. **Power Cycle Count**

12. **Unexpected Power Loss**

13. **Spin-Up Time**

14. **Spin-up Retries**

15. **Calibration Retries**

16. **Temperature**

17. **Reallocated Sectors Count**

18. **Reserved Block Count**

19. **Program Fail Count**

20. **Erase Fail Count**

21. **Wear Leveller Worst Case Erase Count**

22. **Unused Reserved NAND Blocks**

23. **Reallocation Event Count**

24. **Current Pending Sector Count**

25. **Offline Uncorrectable Sector Count**

26. **Percent Lifetime Used**

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
