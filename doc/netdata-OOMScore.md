# OOMScore

netdata runs with `OOMScore = 1000`. This means netdata will be the first to be killed when your server runs out of memory.

## setting netdata OOMScore

You can set netdata OOMScore in `netdata.conf`, like this:

```
[global]
    OOM score = 1000
```

netdata logs its OOM score when it starts:

```sh
# grep OOM /var/log/netdata/error.log
2017-10-15 03:47:31: netdata INFO : Adjusted my Out-Of-Memory (OOM) score from 0 to 1000.
```

#### OOM score and systemd

netdata will not be able to lower its OOM Score below zero, when it is started as the `netdata` user (systemd case).

To allow netdata control its OOM Score in such cases, you will need to edit `/etc/systemd/system/netdata.service` and set:

```
[Service]
# The minimum netdata Out-Of-Memory (OOM) score.
# netdata (via [global].OOM score in netdata.conf) can only increase the value set here.
# To decrease it, set the minimum here and set the same or a higher value in netdata.conf.
# Valid values: -1000 (never kill netdata) to 1000 (always kill netdata).
OOMScoreAdjust=-1000
```

Run `systemctl daemon-reload` to reload these changes.

The above, sets and OOMScore for netdata to `-1000`, so that netdata can increase it via `netdata.conf`.

If you want to control it entirely via systemd, you can set in `netdata.conf`:

```
[global]
    OOM score = keep
```

Using the above, whatever OOM Score you have set at `netdata.service` will be maintained by netdata.
