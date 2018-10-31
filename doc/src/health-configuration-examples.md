# Health Configuration Examples

Check the **[health.d directory](https://github.com/netdata/netdata/tree/master/conf.d/health.d)** for all alarms shipped with netdata.

Here are a few examples:

## Example 1: Check if an apache server is alive

```
template: apache_last_collected_secs
      on: apache.requests
    calc: $now - $last_collected_t
   every: 10s
    warn: $this > ( 5 * $update_every)
    crit: $this > (10 * $update_every)
```

The above checks that netdata is able to collect data from apache. In detail:

```
template: apache_last_collected_secs
```

The above defines a **template** named `apache_last_collected_secs`. The name is important since `$apache_last_collected_secs` resolves to the `calc` line. So, try to give something descriptive.

```
      on: apache.requests
```

The above applies the **template** to all charts that have `context = apache.requests` (i.e. all your apache servers).

```
    calc: $now - $last_collected_t
```

`$now` is a standard variable that resolves to the current timestamp.
`$last_collected_t` is the last data collection timestamp of the chart. So this calculation gives the number of seconds passed since the last data collection.

```
   every: 10s
```

The alarm will be evaluated every 10 seconds.

```
    warn: $this > ( 5 * $update_every)
    crit: $this > (10 * $update_every)
```

If these result in non-zero or true, they trigger the alarm.

`$this` refers to the value of this alarm (i.e. the result of the `calc` line. We could also use `$apache_last_collected_secs`.

`$update_every` is the update frequency of the chart, in seconds.

So, the warning condition checks if we have not collected data from apache for 5 iterations and the critical condition checks for 10 iterations.

## Example 2: Check if any disk's space is critically low

```
template: disk_full_percent
      on: disk.space
    calc: $used * 100 / ($avail + $used)
   every: 1m
    warn: $this > 80
    crit: $this > 95
```

`$used` and `$avail`  are the `used` and `avail` chart dimensions as shown on the dashboard.

So, the `calc` line finds the percentage of used space. `$this` resolves to this percentage.

## Example 3: Predict if any disk will run out of space in the near future.

We do this in 2 steps:

1. Calculate the disk fill rate

  ```
    template: disk_fill_rate
          on: disk.space
      lookup: max -1s at -30m unaligned of avail
        calc: ($this - $avail) / (30 * 60)
       every: 15s
   ```

  In the `calc` line: `$this` is the result of the `lookup` line (i.e. the free space 30 minutes ago) and `$avail` is the current disk free space. So the `calc` line will either have a positive number of GB/second if the disk if filling up, or a negative number of GB/second if the disk is freeing up space.

  There is no `warn` or `crit` lines here. So, this template will just do the calculation and nothing more.

2. Predict the hours after which the disk will run out of space

   ```
    template: disk_full_after_hours
          on: disk.space
        calc: $avail / $disk_fill_rate / 3600
       every: 10s
        warn: $this > 0 and $this < 48
        crit: $this > 0 and $this < 24
   ```

  the `calc` line estimates the time in hours, we will run out of disk space. Of course, only positive values are interesting for this check, so the warning and critical conditions check for positive values and that we have enough free space for 48 and 24 hours respectively.

  Once this alarm triggers we will receive an email like this:

  ![image](https://cloud.githubusercontent.com/assets/2662304/17839993/87872b32-6802-11e6-8e08-b2e4afef93bb.png)

## Example 4: Check if any network interface is dropping packets

```
template: 30min_packet_drops
      on: net.drops
  lookup: sum -30m unaligned absolute
   every: 10s
    crit: $this > 0
```

The `lookup` line will calculate the sum of the all dropped packets in the last 30 minutes.

The `crit` line will issue a critical alarm if even a single packet has been dropped.

Note that the drops chart does not exist if a network interface has never dropped a single packet. When netdata detects a dropped packet, it will add the chart and it will automatically attach this alarm to it.
