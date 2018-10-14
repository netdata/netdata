> THIS MODULE IS OBSOLETE.
> USE THE PYTHON ONE - IT SUPPORTS MULTIPLE JOBS AND IT IS MORE EFFICIENT

# hddtemp

The plugin will collect temperatures from disks 

It will create one chart with all active disks

1. **temperature in Celsius**

### configuration

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
