# Windows.plugin

This internal plugin is only available for Microsoft Windows operating systems.

## The Collector

This plugin primarily collects metrics from Microsoft Windows [Performance Counters](https://learn.microsoft.com/en-us/windows/win32/perfctrs/performance-counters-what-s-new). All detected metrics are automatically displayed without requiring additional configuration.
=======
Most of the metrics collected by this plugin originate from Microsoft Windows
[Performance Counters](https://learn.microsoft.com/en-us/windows/win32/perfctrs/performance-counters-what-s-new).

These metrics are always displayed when detected, requiring no additional configuration.

By default, all collector threads are enabled except for `PerflibThermalZone` and `PerflibServices`. You can enable these or disable others by modifying options in the `[plugin:windows]` section of the configuration file.

To change a setting, remove the comment symbol (`#`) from the beginning of the line and set the value to either `yes` or `no`:

```text
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
        # PerflibServices = no
        # PerflibMSSQL = yes
        # PerflibNetFramework = yes
        # PerflibAD = yes
        # PerflibADCS = yes
        # PerflibADFS = yes
```

## Configuration

To collect certain metrics for [Microsoft SQL Server](https://www.microsoft.com/en-us/sql-server),
Netdata needs access to internal server data. To collect this data, you need to configure both your server and Netdata.

### Windows Defender Firewall with Advanced Security

To connect to your SQL Server, Netdata uses TCP port 1433 (or any other configured port).
This port is typically blocked by Windows Defender, so you must allow access before proceeding:

1. Open `Windows Defender Firewall with Advanced Security` as an Adminstrator.
2. Right-click `Inbound Rules` and select `New Rule...`.
3. Select `Port`, then click `Next`.
4. Choose `TCP`, then enter `1433` in the `Specific local ports:`. Click `Next`.
5. Select an appropriate action based on your network policy. For example, `Allow the connection`.
   Click `Next`.
6. Choose where the rule will be applied (Domain, Private, or Public).
7. Finally, provide a `Name` and an optional `Description` for the new rule,
   then click `Finish`.

### Microsoft SQL Server (Configuration)

After enabling access through the firewall, you will need to configure your SQL Server instance to accept TCP
connections:

1. Open `SQL Server Configuration Manager`.
2. Expand `SQL Server Network Configuration`.
3. In the console panel, select `Protocols for <instance name>.`.
4. In the details panel, double-click the `TCP` protocol, and set `Yes` for `Enabled`.
5. Go to the `IP Address` tab, locate the section labeled`IPAII`.
   - Remove any value from the `TCP Dynamic Ports` field.
   - Enter a value in the `TCP Port` field. By default, SQL Server uses `1433`.
6. Finally, select `SQL Server Services`, and restart your SQL Server instance.

### Microsoft SQL Server (User)

Netdata requires a user with the `VIEW SERVER STATE` permission to collect data from the server.
Once the user is created, it must be added to the databases.

```sql
USE master;
CREATE LOGIN netdata_user WITH PASSWORD = 'netdata';
CREATE USER netdata_user FOR LOGIN netdata_user;
GRANT CONNECT SQL TO netdata_user;
GRANT VIEW SERVER STATE TO netdata_user;
```

In addition to creating the user, you must enable the
[query store](https://learn.microsoft.com/en-us/sql/relational-databases/performance/monitoring-performance-by-using-the-query-store?view=sql-server-ver16),
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

By default, Microsoft SQL Server is installed with "integrated authentication only".
If you want to allow connections using SQL Server authentication, you must modify the server configuration:

1. Open `Microsoft Server Management Studio`.
2. Right-click your server, and select `Properties`.
3. In the left panel, select `Security`.
4. Under `Server authentication`, choose `SQL Server and Windows Authentication mode`.
5. Click `OK`.
6. Finally, right-click your server, and select`Restart`.

### Netdata Configuration

Now that the user has been created inside your server, update `netdata.conf` by adding the following section:

```text
[plugin:windows:PerflibMSSQL]
        driver = SQL Server
        server = 127.0.0.1\\Dev, 1433
        #address = [protocol:]Address[,port |\pipe\pipename]
        uid = netdata_user
        pwd = AReallyStrongPasswordShouldBeInsertedHere
        #windows authentication = no
```

For additional information on how to set these parameters, refer to the
[Microsoft Official Documentation](https://learn.microsoft.com/en-us/sql/relational-databases/native-client/applications/using-connection-string-keywords-with-sql-server-native-client?view=sql-server-ver15&viewFallbackFrom=sql-server-ver16)

When configuring Netdata to access SQL Server, some errors may occur. You can check your configuration using the
Microsoft Event Viewer by looking for entries that begin with `MSSQL server error using the handle`.

#### Data source name not found and no default driver

This error occurs when the driver is not specified correctly. You can check the available drivers by following
these steps:

- Access `ODBC Data Sources`.
- Go to `Drivers` tab.
- Look for `Name` of the `ODBC` or `SQL Server` driver .
