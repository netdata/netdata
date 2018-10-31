# Netdata process priority

By default netdata runs with the `idle` process scheduling policy, so that it uses CPU resources, only when there is idle CPU to spare. On very busy servers (or weak servers), this can lead to gaps on the charts.

## setting netdata process scheduling policy

You can set netdata scheduling policy in `netdata.conf`, like this:

```
[global]
  process scheduling policy = idle
```

You can use the following:

policy|description
:-----:|:--------
`idle`|use CPU only when there is spare - this is lower than nice 19 - it is the default for netdata and it is so low that netdata will run in "slow motion" under extreme system load, resulting in short (1-2 seconds) gaps at the charts.
`other`<br/>or<br/>`nice`|this is the default policy for all processes under Linux. It provides dynamic priorities based on the `nice` level of each process. Check below for setting this `nice` level for netdata.
`batch`|This policy is similar to `other` in that it schedules the thread according to its dynamic priority (based on the `nice` value).  The difference is that this policy will cause the scheduler to always assume that the thread is CPU-intensive.  Consequently, the scheduler will  apply a small scheduling penalty with respect to wake-up behavior, so that this thread is mildly disfavored in scheduling decisions.
`fifo`|`fifo` can be used only with static priorities higher than 0, which means that when a `fifo` threads becomes runnable, it will always  immediately  preempt  any  currently running  `other`, `batch`, or `idle` thread.  `fifo` is a simple scheduling algorithm without time slicing.
`rr`|a simple enhancement of `fifo`.  Everything described above for `fifo` also applies to `rr`, except that each thread is allowed to run only for a  maximum time quantum.
`keep`<br/>or<br/>`none`|do not set scheduling policy, priority or nice level - i.e. keep running with whatever it is set already (e.g. by systemd).

For more information see `man sched`.

### scheduling priority for `rr` and `fifo`

Once the policy is set to one of `rr` or `fifo`, the following will appear:

```
[global]
    process scheduling priority = 0
```

These priorities are usually from 0 to 99. Higher numbers make the process more important.

### nice level for policies `other` or `batch`

When the policy is set to `other`, `nice`, or `batch`, the following will appear:

```
[global]
    process nice level = 19
```

## scheduling settings and systemd

netdata will not be able to set its scheduling policy and priority to more important values when it is started as the `netdata` user (systemd case).

You can set these settings at `/etc/systemd/system/netdata.service`:

```
[Service]
# By default netdata switches to scheduling policy idle, which makes it use CPU, only
# when there is spare available.
# Valid policies: other (the system default) | batch | idle | fifo | rr
#CPUSchedulingPolicy=other

# This sets the maximum scheduling priority netdata can set (for policies: rr and fifo).
# netdata (via [global].process scheduling priority in netdata.conf) can only lower this value.
# Priority gets values 1 (lowest) to 99 (highest).
#CPUSchedulingPriority=1

# For scheduling policy 'other' and 'batch', this sets the lowest niceness of netdata.
# netdata (via [global].process nice level in netdata.conf) can only increase the value set here.
#Nice=0
```

Run `systemctl daemon-reload` to reload these changes.

Now, tell netdata to keep these settings, as set by systemd, by editing `netdata.conf` and setting:

```
[global]
    process scheduling policy = keep
```

Using the above, whatever scheduling settings you have set at `netdata.service` will be maintained by netdata.


## Example 1: netdata with nice -1 on non-systemd systems

On a system that is not based on systemd, to make netdata run with nice level -1 (a little bit higher to the default for all programs), edit netdata.conf and set:

```
[global]
  process scheduling policy = other
  process nice level = -1
```

then execute this to restart netdata:

```sh
sudo service netdata restart
```


## Example 2: netdata with nice -1 on systemd systems

On a system that is based on systemd, to make netdata run with nice level -1 (a little bit higher to the default for all programs), edit netdata.conf and set:

```
[global]
  process scheduling policy = keep
```

edit /etc/systemd/system/netdata.service and set:

```
[Service]
CPUSchedulingPolicy=other
Nice=-1
```

then execute:

```sh
sudo systemctl daemon-reload
sudo systemctl restart netdata
```
