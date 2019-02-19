# oracledb

Module monitor oracledb performance and health metrics.

**Requirements:**
 - `cx_Oracle` package.

It produces following charts:
 - session activity
   - Session Count
   - Session Limit Usage
   - Logons
 - disk activity
   - Physical Disk Reads/Writes
   - Sorts On Disk
   - Full Table Scans
 - database and buffer activity
   - Database Wait Time Ratio
   - Shared Pool Free Memory
   - In-Memory Sorts Ratio
   - SQL Service Response Time
   - User Rollbacks
   - Enqueue Timeouts
 - cache
   - Cache Hit Ratio
 - activities
   - Activities
 - wait time
   - Wait Time
 - tablespace
   - Size
   - Usage
   - Usage In Percent

### prerequisite

### configuration

```yaml
local:
  user: 'netdata'
  password: 'secret'
  server: 'localhost:1521'
  service: 'XE'

remote:
  user: 'netdata'
  password: 'secret'
  server: '10.0.0.1:1521'
  service: 'XE'
```

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fcollectors%2Fpython.d.plugin%2Fexample%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)]()
