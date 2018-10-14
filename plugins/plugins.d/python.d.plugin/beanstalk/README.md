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
