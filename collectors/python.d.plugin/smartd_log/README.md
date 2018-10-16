# smartd_log

Module monitor `smartd` log files to collect HDD/SSD S.M.A.R.T attributes.

It produces following charts (you can add additional attributes in the module configuration file):

1. **Read Error Rate** attribute 1

2. **Start/Stop Count** attribute 4

3. **Reallocated Sectors Count** attribute 5

4. **Seek Error Rate** attribute 7

5. **Power-On Hours Count** attribute 9

6. **Power Cycle Count** attribute 12

7. **Load/Unload Cycles** attribute 193

8. **Temperature** attribute 194

9. **Current Pending Sectors** attribute 197

10. **Off-Line Uncorrectable** attribute 198

11. **Write Error Rate** attribute 200

### configuration

```yaml
local:
  log_path : '/var/log/smartd/'
```

If no configuration is given, module will attempt to read log files in /var/log/smartd/ directory.

---
