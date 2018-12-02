# proxysql

This module monitors proxysql backend and frontend performance metrics.

It produces:

1. **Connections (frontend)**
  * connected: number of frontend connections currently connected
  * aborted: number of frontend connections aborted due to invalid credential or max_connections reached
  * non_idle: number of frontend connections that are not currently idle
  * created: number of frontend connections created
2. **Questions (frontend)**
  * questions: total number of queries sent from frontends
  * slow_queries: number of queries that ran for longer than the threshold in milliseconds defined in global variable `mysql-long_query_time`
3. **Overall Bandwith (backends)**
  * in
  * out
4. **Status (backends)**
  * Backends
    * `1=ONLINE`: backend server is fully operational
    * `2=SHUNNED`: backend sever is temporarily taken out of use because of either too many connection errors in a time that was too short, or replication lag exceeded the allowed threshold
    * `3=OFFLINE_SOFT`: when a server is put into OFFLINE_SOFT mode, new incoming connections aren't accepted anymore, while the existing connections are kept until they became inactive. In other words, connections are kept in use until the current transaction is completed. This allows to gracefully detach a backend
    * `4=OFFLINE_HARD`: when a server is put into OFFLINE_HARD mode, the existing connections are dropped, while new incoming connections aren't accepted either. This is equivalent to deleting the server from a hostgroup, or temporarily taking it out of the hostgroup for maintenance work
    * `-1`: Unknown status
5. **Bandwith (backends)**
  * Backends
    * in
    * out
6. **Queries (backends)**
  * Backends
    * queries
7. **Latency (backends)**
  * Backends
    * ping time
8. **Pool connections (backends)**
  * Backends
    * Used: The number of connections are currently used by ProxySQL for sending queries to the backend server.
    * Free: The number of connections are currently free.
    * Established/OK: The number of connections were established successfully.
    * Error: The number of connections weren't established successfully.
9. **Commands**
  * Commands
    * Count
    * Duration (Total duration for each command)
10. **Commands Histogram**
  * Commands
    * 100us, 500us, ..., 10s, inf: the total number of commands of the given type which executed within the specified time limit and the previous one.

### configuration

```yaml
tcpipv4:
  name     : 'local'
  user     : 'stats'
  pass     : 'stats'
  host     : '127.0.0.1'
  port     : '6032'
```

If no configuration is given, module will fail to run.

---
