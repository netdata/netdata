<!--
---
title: "MySQL monitoring with Netdata"
custom_edit_url: https://github.com/netdata/netdata/edit/master/collectors/python.d.plugin/mysql/README.md
---
-->

# MySQL monitoring with Netdata

Monitors one or more MySQL servers.

## Requirements

-   python library [MySQLdb](https://github.com/PyMySQL/mysqlclient-python) (faster) or [PyMySQL](https://github.com/PyMySQL/PyMySQL) (slower)
-   `netdata` local user to connect to the MySQL server. 

To create the `netdata` user, execute the following in the MySQL shell:

```sh
create user 'netdata'@'localhost';
grant usage on *.* to 'netdata'@'localhost';
flush privileges;
```
The `netdata` user will have the ability to connect to the MySQL server on `localhost` without a password. 
It will only be able to gather MySQL statistics without being able to alter or affect MySQL operations in any way. 

This module will produce following charts (if data is available):

1.  **Bandwidth** in kilobits/s

    -   in
    -   out

2.  **Queries** in queries/sec

    -   queries
    -   questions
    -   slow queries

3.  **Queries By Type** in queries/s

    -   select
    -   delete
    -   update
    -   insert
    -   cache hits
    -   replace

4.  **Handlerse** in handlers/s

    -   commit
    -   delete
    -   prepare
    -   read first
    -   read key
    -   read next
    -   read prev
    -   read rnd
    -   read rnd next
    -   rollback
    -   savepoint
    -   savepoint rollback
    -   update
    -   write

5.  **Table Locks** in locks/s

    -   immediate
    -   waited

6.  **Table Select Join Issuess** in joins/s

    -   full join
    -   full range join
    -   range
    -   range check
    -   scan

7.  **Table Sort Issuess** in joins/s

    -   merge passes
    -   range
    -   scan

8.  **Tmp Operations** in created/s

    -   disk tables
    -   files
    -   tables

9.  **Connections** in connections/s

    -   all
    -   aborted

10. **Connections Active** in connections/s

    -   active
    -   limit
    -   max active

11. **Binlog Cache** in threads

    -   disk
    -   all

12. **Threads** in transactions/s

    -   connected
    -   cached
    -   running

13. **Threads Creation Rate** in threads/s

    -   created

14. **Threads Cache Misses** in misses

    -   misses

15. **InnoDB I/O Bandwidth** in KiB/s

    -   read
    -   write

16. **InnoDB I/O Operations** in operations/s

    -   reads
    -   writes
    -   fsyncs

17. **InnoDB Pending I/O Operations** in operations/s

    -   reads
    -   writes
    -   fsyncs

18. **InnoDB Log Operations** in operations/s

    -   waits
    -   write requests
    -   writes

19. **InnoDB OS Log Pending Operations** in operations

    -   fsyncs
    -   writes

20. **InnoDB OS Log Operations** in operations/s

    -   fsyncs

21. **InnoDB OS Log Bandwidth** in KiB/s

    -   write

22. **InnoDB Current Row Locks** in operations

    -   current waits

23. **InnoDB Row Operations** in operations/s

    -   inserted
    -   read
    -   updated
    -   deleted

24. **InnoDB Buffer Pool Pagess** in pages

    -   data
    -   dirty
    -   free
    -   misc
    -   total

25. **InnoDB Buffer Pool Flush Pages Requests** in requests/s

    -   flush pages

26. **InnoDB Buffer Pool Bytes** in MiB

    -   data
    -   dirty

27. **InnoDB Buffer Pool Operations** in operations/s

    -   disk reads
    -   wait free

28. **QCache Operations** in queries/s

    -   hits
    -   lowmem prunes
    -   inserts
    -   no caches

29. **QCache Queries in Cache** in queries

    -   queries

30. **QCache Free Memory** in MiB

    -   free

31. **QCache Memory Blocks** in blocks

    -   free
    -   total

32. **MyISAM Key Cache Blocks** in blocks

    -   unused
    -   used
    -   not flushed

33. **MyISAM Key Cache Requests** in requests/s

    -   reads
    -   writes

34. **MyISAM Key Cache Requests** in requests/s

    -   reads
    -   writes

35. **MyISAM Key Cache Disk Operations** in operations/s

    -   reads
    -   writes

36. **Open Files** in files

    -   files

37. **Opened Files Rate** in files/s

    -   files

38. **Binlog Statement Cache** in statements/s

    -   disk
    -   all

39. **Connection Errors** in errors/s

    -   accept
    -   internal
    -   max
    -   peer addr
    -   select
    -   tcpwrap

40. **Slave Behind Seconds** in seconds

    -   time

41. **I/O / SQL Thread Running State** in bool

    -   sql
    -   io

42. **Galera Replicated Writesets** in writesets/s

    -   rx
    -   tx

43. **Galera Replicated Bytes** in KiB/s

    -   rx
    -   tx

44. **Galera Queue** in writesets

    -   rx
    -   tx

45. **Galera Replication Conflicts** in transactions

    -   bf aborts
    -   cert fails

46. **Galera Flow Control** in ms

    -   paused

47. **Galera Cluster Status** in status

    -   status

48. **Galera Cluster State** in state

    -   state

49. **Galera Number of Nodes in the Cluster** in num

    -   nodes

50. **Galera Total Weight of the Current Members in the Cluster** in weight

    -   weight

51. **Galera Whether the Node is Connected to the Cluster** in boolean

    -   connected

52. **Galera Whether the Node is Ready to Accept Queries** in boolean

    -   ready

53. **Galera Open Transactions** in num

    -   open transactions

54. **Galera Total Number of WSRep (applier/rollbacker) Threads** in num

    -   threads

55. **Users CPU time** in percentage

    -   users

**Per user statistics:**

1.  **Rows Operations** in operations/s

    -   read
    -   send
    -   updated
    -   inserted
    -   deleted

2.  **Commands** in commands/s

    -   select
    -   update
    -   other

## Configuration

Edit the `python.d/mysql.conf` configuration file using `edit-config` from the your agent's [config
directory](../../../docs/step-by-step/step-04.md#find-your-netdataconf-file), which is typically at `/etc/netdata`.

```bash
cd /etc/netdata   # Replace this path with your Netdata config directory, if different
sudo ./edit-config python.d/mysql.conf
```

You can provide, per server, the following:

1.  username which have access to database (defaults to 'root')
2.  password (defaults to none)
3.  mysql my.cnf configuration file
4.  mysql socket (optional)
5.  mysql host (ip or hostname)
6.  mysql port (defaults to 3306)
7.  ssl connection parameters

    -   key: the path name of the client private key file.
    -   cert: the path name of the client public key certificate file.
    -   ca: the path name of the Certificate Authority (CA) certificate file. This option, if used, must specify the
        same certificate used by the server.
    -   capath: the path name of the directory that contains trusted SSL CA certificate files.
    -   cipher: the list of permitted ciphers for SSL encryption.

Here is an example for 3 servers:

```yaml
update_every : 10
priority     : 90100

local:
  'my.cnf'   : '/etc/mysql/my.cnf'
  priority   : 90000

local_2:
  user     : 'root'
  pass : 'blablablabla'
  socket   : '/var/run/mysqld/mysqld.sock'
  update_every : 1

remote:
  user     : 'admin'
  pass : 'bla'
  host     : 'example.org'
  port     : 9000
```

If no configuration is given, the module will attempt to connect to MySQL server via a unix socket at
`/var/run/mysqld/mysqld.sock` without password and with username `root`.

`userstats` graph works only if you enable the plugin in MariaDB server and set proper MySQL privileges (SUPER or
PROCESS). For more details, please check the [MariaDB User Statistics
page](https://mariadb.com/kb/en/library/user-statistics/)

---

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fcollectors%2Fpython.d.plugin%2Fmysql%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
