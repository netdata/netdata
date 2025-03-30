# Windows.plugin

This is an internal plugin available only for Microsoft Windows operating systems.

## The Collector

Most of the metrics collected by this plugin originate from Microsoft Windows [Performance Counters](https://learn.microsoft.com/en-us/windows/win32/perfctrs/performance-counters-what-s-new).
These metrics are always displayed when detected, requiring no additional configuration.

## Configuration File

The Windows plugin does not have a separate configuration file. To configure it, use Netdata’s default configuration file:
`C:\Program Files\Netdata\etc\netdata\netdata.conf`.

### Enabling or Disabling Collection

With the exception of the `PerflibThermalZone` thread, all other threads are enabled by default. You can enable it
or disable others by modifying a specific option for a thread inside the `[plugin:windows]` section. To do this,
remove the comment symbol (`#`) at the beginning of the line and then change the boolean value after the equals sign.

``` 
[plugin:windows]
        # GetSystemUptime = yes
        # GetSystemRAM = yes
        # PerflibProcesses = yes
        # PerflibProcessor = yes
        # PerflibMemory = yes
        # PerflibStorage = yes
        # PerflibNetwork = yes
        # PerflibObjects = yes
        # PerflibHyperV = yes
        # PerflibThermalZone = no
        # PerflibWebService = yes
        # PerflibMSSQL = yes
        # PerflibNetFramework = yes
        # PerflibAD = yes
        # PerflibADCS = yes
        # PerflibADFS = yes
```

### MSSQL

To collect certain metrics for [Microsoft SQL Server](https://www.microsoft.com/en-us/sql-server), Netdata needs
access to internal server data.

#### Windows Defender Firewall with Advanced Security

To connect to your SQL Server, Netdata uses TCP port 1433 (or any other configured port).
This port is typically blocked by Windows Defender, so you must allow access before proceeding:

1. Open `Windows Defender Firewall with Advanced Security` as an Adminstrator.
2. Right-click `Inbound Rules` and select `New Rule...`.
3. Select `Port`, then click `Next`.
4. Choose `TCP`, then enter `1433` in the `Specific local ports:`. Click `Next`.
5. Select an appropriate action based on your network policy. For example, `Allow the connection`. Click `Next`.
6. Choose where the rule will be applied (Domain, Private, or Public).
7. Finally, provide a `Name` and an optional `Description` for the new rule, then click `Finish`.

#### Microsoft SQL Server

Netdata requires a user with the `VIEW SERVER STATE` permission to collect data from the server.
Once the user is created, it must be added to the databases.

```sql
USE master;
CREATE LOGIN netdata_user WITH PASSWORD = 'netdata';
CREATE USER netdata_user FOR LOGIN netdata_user;
GRANT CONNECT SQL TO netdata_user;
GRANT VIEW SERVER STATE TO netdata_user;
```

In addition to creating the user, you must enable the [query store](https://learn.microsoft.com/en-us/sql/relational-databases/performance/monitoring-performance-by-using-the-query-store?view=sql-server-ver16),
on each database you wish to monitor.

```sql
USE master;
CREATE LOGIN netdata_user WITH PASSWORD = 'AReallyStrongPasswordShouldBeInsertedHere';
CREATE USER netdata_user FOR LOGIN netdata_user;
GRANT CONNECT SQL TO netdata_user;
GRANT VIEW SERVER STATE TO netdata_user;

DECLARE @dbname NVARCHAR(max)
DECLARE nd_user_cursor CURSOR FOR SELECT name FROM master.dbo.sysdatabases WHERE NAME NOT IN ('master','msdb','tempdb','model')

OPEN nd_user_cursor
FETCH NEXT FROM nd_user_cursor INTO @dbname WHILE @@FETCH_STATUS = 0
BEGIN
        EXECUTE ("USE "+ @dbname+"; CREATE USER netdata_user FOR LOGIN netdata_user; ALTER DATABASE "+@dbname+" SET QUERY_STORE = ON ( QUERY_CAPTURE_MODE = ALL, DATA_FLUSH_INTERVAL_SECONDS = 900 )");
        FETCH next FROM nd_user_cursor INTO @dbname;
END
CLOSE nd_user_cursor
DEALLOCATE nd_user_cursor
GO
```
#### Netdata Configuration

Now that the user has been created inside your server, update `netdata.conf` by adding the following section:

```
[plugin:windows:PerflibMSSQL]
        username = netdata_user
        password = AReallyStrongPasswordShouldBeInsertedHere
        hostname = localhost
        port = 1433
```
