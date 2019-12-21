# hpssa

This module collects controller, logical, and physical device health metrics from HP Smart Storage Arrays.

## Requirements:

- `ssacli` program
- `sudo` program
- `netdata` user needs to be able to sudo the `ssacli` program without password

To collect metrics, the module executes: `sudo -n ssacli ctrl all show config detail`

This module produces:

1.  Controller state and temperature
2.  Cache module state and temperature
3.  Logical drive state
4.  Physical drive state and temperature

## Prerequisite

This module uses `ssacli`, which can only be executed by root. It uses
`sudo` and assumes that it is configured such that the `netdata` user can
execute `ssacli` as root without password.

Add to `sudoers`:

```
netdata ALL=(root)       NOPASSWD: /path/to/ssacli
```

## Configuration

**hpssa** is disabled by default. Should be explicitly enabled in `python.d.conf`.

```yaml
hpssa: yes
```

If `ssacli` cannot be found in the `PATH`, configure it in `hpssa.conf`.

```yaml
ssacli_path: /usr/sbin/ssacli
```
