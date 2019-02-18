# beanstalk

Module provides server and tube-level statistics:

**Requirements:**
 * `python-beanstalkc`

**Server statistics:**

1. **Cpu usage** in cpu time
 * user
 * system

2. **Jobs rate** in jobs/s
 * total
 * timeouts

3. **Connections rate** in connections/s
 * connections

4. **Commands rate** in commands/s
 * put
 * peek
 * peek-ready
 * peek-delayed
 * peek-buried
 * reserve
 * use
 * watch
 * ignore
 * delete
 * release
 * bury
 * kick
 * stats
 * stats-job
 * stats-tube
 * list-tubes
 * list-tube-used
 * list-tubes-watched
 * pause-tube

5. **Current tubes** in tubes
 * tubes

6. **Current jobs** in jobs
 * urgent
 * ready
 * reserved
 * delayed
 * buried

7. **Current connections** in connections
 * written
 * producers
 * workers
 * waiting

8. **Binlog** in records/s
 * written
 * migrated

9. **Uptime** in seconds
 * uptime

**Per tube statistics:**

1. **Jobs rate** in jobs/s
 * jobs

2. **Jobs** in jobs
 * using
 * ready
 * reserved
 * delayed
 * buried

3. **Connections** in connections
 * using
 * waiting
 * watching

4. **Commands** in commands/s
 * deletes
 * pauses

5. **Pause** in seconds
 * since
 * left


### configuration

Sample:

```yaml
host         : '127.0.0.1'
port         : 11300
```

If no configuration is given, module will attempt to connect to beanstalkd on `127.0.0.1:11300` address

---

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fcollectors%2Fpython.d.plugin%2Fbeanstalk%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)]()
