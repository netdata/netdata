# Windows.plugin

This internal plugin is only available for Microsoft Windows operating systems.

## The Collector

This plugin primarily collects metrics from Microsoft Windows [Performance Counters](https://learn.microsoft.com/en-us/windows/win32/perfctrs/performance-counters-what-s-new). All detected metrics are automatically displayed without requiring additional configuration.
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

You can use the same rule to allow connections to other instances. To do this,
specify the ports for each instance in the `Port` field separated by commas.

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

You need to apply this configuration to every instance.

### Microsoft SQL Server (User)

If you want to allow connections using SQL Server authentication, you must modify the server configuration:

1. Open `Microsoft Server Management Studio`.
2. Right-click your server, and select `Properties`.
3. In the left panel, select `Security`.
4. Under `Server authentication`, choose `SQL Server and Windows Authentication mode`.
5. Click `OK`.
6. Finally, right-click your server, and select`Restart`.

After that, you need to create a user with the `VIEW SERVER STATE` permission to collect data from the server.
Once the user is created, it must also be granted access to the databases.

```tsql
USE master;
CREATE LOGIN netdata_user WITH PASSWORD = 'AReallyStrongPasswordShouldBeInsertedHere';
CREATE USER netdata_user FOR LOGIN netdata_user;
GRANT CONNECT SQL TO netdata_user;
GRANT VIEW SERVER STATE TO netdata_user;
GO
```

In addition to creating the user, you must enable the
[query store](https://learn.microsoft.com/en-us/sql/relational-databases/performance/monitoring-performance-by-using-the-query-store?view=sql-server-ver16),
on each database you wish to monitor.

```tsql
DECLARE @dbname NVARCHAR(max)
DECLARE nd_user_cursor CURSOR FOR SELECT name FROM master.dbo.sysdatabases WHERE name NOT IN ('master', 'tempdb')

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

You need to apply this configuration to every instance.

### Netdata Configuration

Now that the user has been created inside your server, update `netdata.conf` by adding the following section:

```text
[plugin:windows:PerflibMSSQL]
        driver = SQL Server
        server = 127.0.0.1\\Dev, 1433
        #address = [protocol:]Address[,port |\pipe\pipename]
        uid = netdata_user
        pwd = AReallyStrongPasswordShouldBeInsertedHere
        # additional instances = 0
        #windows authentication = no
```

In next table, we give a short description about them:

| Option                   | Description                                                                       |
|--------------------------|-----------------------------------------------------------------------------------|
|`driver`                  | ODBC driver used to connect to the MSSQL server.                                  |
|`server`                  | Server address or instance name to connect to.                                    |
|`address`                 | Similar to `server`. You can also use named pipes if your server supports them.   |
|`uid`                     | User identifier (`uid`) created on your server.                                   |
|`pwd`                     | Password for the specified user.                                                  |
|`additional instances`    | Number of additional MSSQL instances to monitor.                                  |
|`windows authentications` | Use Windows credentials to connect to the MSSQL server.                           |

For additional information on how to set these parameters, refer to the
[Microsoft Official Documentation](https://learn.microsoft.com/en-us/sql/relational-databases/native-client/applications/using-connection-string-keywords-with-sql-server-native-client?view=sql-server-ver15&viewFallbackFrom=sql-server-ver16)

#### Additional Instance

Microsoft SQL Server can host multiple instances. To connect to them, you need to create additional sections in your
`netdata.conf` file, specifying their respective connection options.

Let’s suppose you have an additional instance named `Production`. To enable Netdata to monitor both the `Production`
and `Dev` instances, you’ll need to add the following configuration section:

```text
[plugin:windows:PerflibMSSQL1]
        driver = SQL Server
        server = 127.0.0.1\\Production, 1434
        uid = netdata_user
        pwd = AnotherReallyStrongPasswordShouldBeInsertedHere
```

You must also update the main section to indicate how many additional instances you want to monitor:

```text
[plugin:windows:PerflibMSSQL]
        additional instances = 1
```

Each additional instance section must follow the naming pattern `plugin:windows:PerflibMSSQL`, with a sequential number
from 1 to 65535, based on the number of instances you want to monitor.

### Errors

When configuring Netdata to access SQL Server, some errors may occur. You can check your configuration using the
Microsoft Event Viewer by looking for entries that begin with `MSSQL server error using the handle`.

#### Data source name not found and no default driver

This error occurs when the driver is not specified correctly. You can check the available drivers by following
these steps:

- Access `ODBC Data Sources`.
- Go to `Drivers` tab.
- Look for `Name` of the `ODBC` or `SQL Server` driver .

#### Database Metrics Not Visible

If you have created a database but it is not appearing on the Netdata dashboard,
this may indicate that statistics collection is not enabled for your database.
To resolve this, follow these steps:

1. Open `SQL Server Configuration Manager`.
2. Right-click the database for which you want to enable statistics and select `Properties`.
3. In the left pane, select `Options`.
4. Set `Auto Create Statistics` to `True`.
