# hddtemp

> THIS MODULE IS OBSOLETE.
> USE [THE PYTHON ONE](../../python.d.plugin/hddtemp) - IT SUPPORTS MULTIPLE JOBS AND IT IS MORE EFFICIENT

The plugin will collect temperatures from disks 

It will create one chart with all active disks

1.  **temperature in Celsius**

## configuration

hddtemp needs to be running in daemonized mode

```sh
# host with daemonized hddtemp
hddtemp_host="localhost"

# port on which hddtemp is showing data
hddtemp_port="7634"

# array of included disks
# the default is to include all
hddtemp_disks=()
```

---

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fcollectors%2Fcharts.d.plugin%2Fhddtemp%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
