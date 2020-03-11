<!--
---
title: "Storage devices monitoring with Netdata"
custom_edit_url: https://github.com/netdata/netdata/edit/master/collectors/python.d.plugin/smartd_log/README.md
---
-->

# Storage devices monitoring with Netdata

Monitors `smartd` log files to collect HDD/SSD S.M.A.R.T attributes.

## Requirements

-   `smartmontools`

It produces following charts for SCSI devices:

1.  **Read Error Corrected**

2.  **Read Error Uncorrected**

3.  **Write Error Corrected**

4.  **Write Error Uncorrected**

5.  **Verify Error Corrected**

6.  **Verify Error Uncorrected**

7.  **Temperature**

For ATA devices:

1.  **Read Error Rate**

2.  **Seek Error Rate**

3.  **Soft Read Error Rate**

4.  **Write Error Rate**

5.  **SATA Interface Downshift**

6.  **UDMA CRC Error Count**

7.  **Throughput Performance**

8.  **Seek Time Performance**

9.  **Start/Stop Count**

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

## prerequisite

`smartd` must be running with `-A` option to write smartd attribute information to files.

For this you need to set `smartd_opts` (or `SMARTD_ARGS`, check _smartd.service_ content) in `/etc/default/smartmontools`:

```
# dump smartd attrs info every 600 seconds
smartd_opts="-A /var/log/smartd/ -i 600"
```

You may need to create the smartd directory before smartd will write to it: 

```sh
mkdir -p /var/log/smartd
```

Otherwise, all the smartd `.csv` files may get written to `/var/lib/smartmontools` (default location). See also <https://linux.die.net/man/8/smartd> for more info on the `-A --attributelog=PREFIX` command.

`smartd` appends logs at every run. It's strongly recommended to use `logrotate` for smartd files.

## Configuration

Edit the `python.d/smartd_log.conf` configuration file using `edit-config` from the your agent's [config
directory](../../../docs/step-by-step/step-04.md#find-your-netdataconf-file), which is typically at `/etc/netdata`.

```bash
cd /etc/netdata   # Replace this path with your Netdata config directory, if different
sudo ./edit-config python.d/smartd_log.conf
```

```yaml
local:
  log_path : '/var/log/smartd/'
```

If no configuration is given, module will attempt to read log files in `/var/log/smartd/` directory.

---

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fcollectors%2Fpython.d.plugin%2Fsmartd_log%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
