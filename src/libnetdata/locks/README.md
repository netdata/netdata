# Locks

## How to trace netdata locks

To enable tracing rwlocks in netdata, compile netdata by setting `CFLAGS="-DNETDATA_TRACE_RWLOCKS=1"`, like this:

```
CFLAGS="-O1 -ggdb -DNETDATA_TRACE_RWLOCKS=1"  ./netdata-installer.sh
```

During compilation, the compiler will log:

```
libnetdata/locks/locks.c:105:2: warning: #warning NETDATA_TRACE_RWLOCKS ENABLED - EXPECT A LOT OF OUTPUT [-Wcpp]
  105 | #warning NETDATA_TRACE_RWLOCKS ENABLED - EXPECT A LOT OF OUTPUT
      |  ^~~~~~~
```

Once compiled, netdata will do the following:

Every call to `netdata_rwlock_*()` is now measured in time.

### logging of slow locks/unlocks

If any call takes more than 10 usec, it will be logged like this:

```
RW_LOCK ON LOCK 0x0x7fbe1f2e5190: 4157038, 'ACLK_Query_2' (function build_context_param_list() 99@web/api/formatters/rrd2json.c) WAITED to UNLOCK for 29 usec.
```

The time can be changed by setting this `-DNETDATA_TRACE_RWLOCKS_WAIT_TIME_TO_IGNORE_USEC=20` (or whatever number) to the CFLAGS.

### logging of long hold times

If any lock is holded for more than 10000 usec, it will be logged like this:

```
RW_LOCK ON LOCK 0x0x55a20afc1b20: 4187198, 'ANALYTICS' (function analytics_gather_mutable_meta_data() 532@daemon/analytics.c) holded a 'R' for 13232 usec.
```

The time can be changed by setting this `-DNETDATA_TRACE_RWLOCKS_HOLD_TIME_TO_IGNORE_USEC=20000` (or whatever number) to the CFLAGS.

### logging for probable pauses (predictive)

The library maintains a linked-list of all the lock holders (one entry per thread).  For this linked-list a mutex is used. So every call to the r/w locks now also has  a mutex lock.

If any call is expected to pause the caller (ie the caller is attempting a read lock while there is a write lock in place and vice versa), the library will log something like this:

```
RW_LOCK ON LOCK 0x0x5651c9fcce20: 4190039 'HEALTH' (function health_execute_pending_updates() 661@health/health.c) WANTS a 'W' lock (while holding 1 rwlocks and 1 mutexes).
There are 7 readers and 0 writers are holding the lock:
     => 1: RW_LOCK: process 4190091 'WEB_SERVER[static14]' (function api_v1_data() 526@web/api/web_api_v1.c) is having 1 'R' lock for 709847 usec.
     => 2: RW_LOCK: process 4190079 'WEB_SERVER[static6]' (function api_v1_data() 526@web/api/web_api_v1.c) is having 1 'R' lock for 709869 usec.
     => 3: RW_LOCK: process 4190084 'WEB_SERVER[static10]' (function api_v1_data() 526@web/api/web_api_v1.c) is having 1 'R' lock for 709948 usec.
     => 4: RW_LOCK: process 4190076 'WEB_SERVER[static3]' (function api_v1_data() 526@web/api/web_api_v1.c) is having 1 'R' lock for 710190 usec.
     => 5: RW_LOCK: process 4190092 'WEB_SERVER[static15]' (function api_v1_data() 526@web/api/web_api_v1.c) is having 1 'R' lock for 710195 usec.
     => 6: RW_LOCK: process 4190077 'WEB_SERVER[static4]' (function api_v1_data() 526@web/api/web_api_v1.c) is having 1 'R' lock for 710208 usec.
     => 7: RW_LOCK: process 4190044 'WEB_SERVER[static1]' (function api_v1_data() 526@web/api/web_api_v1.c) is having 1 'R' lock for 710221 usec.
```

And each of the above is paired with a `GOT` log, like this:

```
RW_LOCK ON LOCK 0x0x5651c9fcce20: 4190039 'HEALTH' (function health_execute_pending_updates() 661@health/health.c) GOT a 'W' lock (while holding 2 rwlocks and 1 mutexes).
There are 0 readers and 1 writers are holding the lock:
     => 1: RW_LOCK: process 4190039 'HEALTH' (function health_execute_pending_updates() 661@health/health.c) is having 1 'W' lock for 36 usec.
```

Keep in mind that the lock and log are not atomic. The list of callers is indicative (and sometimes just empty because the original holders of the lock, unlocked it until we had the chance to print their names).

### POSIX compliance check

The library may also log messages about POSIX unsupported cases, like this:

```
RW_LOCK FATAL ON LOCK 0x0x622000109290: 3609368 'PLUGIN[proc]' (function __rrdset_check_rdlock() 10@database/rrdset.c) attempts to acquire a 'W' lock.
But it is not supported by POSIX because: ALREADY HAS THIS LOCK
At this attempt, the task is holding 1 rwlocks and 1 mutexes.
There are 1 readers and 0 writers are holding the lock requested now:
     => 1: RW_LOCK: process 3609368 'PLUGIN[proc]' (function rrdset_done() 1398@database/rrdset.c) is having 1 'R' lock for 0 usec.
```

### nested read locks

When compiled with `-DNETDATA_TRACE_RWLOCKS_LOG_NESTED=1` the library will also detect nested read locks and print them like this:

```
RW_LOCK ON LOCK 0x0x7ff6ea46d190: 4140225 'WEB_SERVER[static14]' (function rrdr_json_wrapper_begin() 34@web/api/formatters/json_wrapper.c) NESTED READ LOCK REQUEST a 'R' lock (while holding 1 rwlocks and 1 mutexes).
There are 5 readers and 0 writers are holding the lock:
   => 1: RW_LOCK: process 4140225 'WEB_SERVER[static14]' (function rrdr_lock_rrdset() 70@web/api/queries/rrdr.c) is having 1 'R' lock for 216667 usec.
   => 2: RW_LOCK: process 4140211 'WEB_SERVER[static6]' (function rrdr_lock_rrdset() 70@web/api/queries/rrdr.c) is having 1 'R' lock for 220001 usec.
   => 3: RW_LOCK: process 4140218 'WEB_SERVER[static8]' (function rrdr_lock_rrdset() 70@web/api/queries/rrdr.c) is having 1 'R' lock for 220001 usec.
   => 4: RW_LOCK: process 4140224 'WEB_SERVER[static13]' (function rrdr_lock_rrdset() 70@web/api/queries/rrdr.c) is having 1 'R' lock for 220001 usec.
   => 5: RW_LOCK: process 4140227 'WEB_SERVER[static16]' (function rrdr_lock_rrdset() 70@web/api/queries/rrdr.c) is having 1 'R' lock for 220001 usec.
```



