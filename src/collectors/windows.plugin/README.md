# Windows.plugin

This internal plugin is exclusively available for Microsoft Windows operating systems.

## Overview

The Windows plugin primarily collects metrics from Microsoft Windows [Performance Counters](https://learn.microsoft.com/en-us/windows/win32/perfctrs/performance-counters-what-s-new). All detected metrics are automatically displayed in the Netdata dashboard without requiring additional configuration.

## Default Configuration

By default, all collector threads are enabled except for `PerflibThermalZone` and `PerflibServices`. You can enable these disabled collectors or disable any of the currently active ones by modifying the `[plugin:windows]` section in your configuration file.

To change a setting, remove the comment symbol (`#`) from the beginning of the line and set the value to either `yes` or `no`.

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
        # PerflibExchange = yes
```

## Update Every per Thread

When the plugin is running, most threads will collect data using Netdata’s default update `every interval`. However,
to avoid overloading the host, Netdata uses different `update every` intervals for specific threads, as shown below:

| Period (seconds) | Threads                                       |
|------------------|-----------------------------------------------|
| 5                | `HyperV`                                      |
| 10               | `MSSQL`, `AD`, `ADCS`, `ADFS`, and `Exchange` |
| 30               | `Services`                                    |

To customize the update interval for a specific thread, you can set the update every value within the corresponding
thread configuration in your `netdata.conf` file. For example, to modify the intervals for the `Object`
and `HyperV` threads:

```text
[plugin:windows:PerflibObjects]
        #update every = 10s

[plugin:windows:PerflibHyperV]
        #update every = 15s
```

## Microsoft SQL Server Integration

To collect metrics from [Microsoft SQL Server](https://www.microsoft.com/en-us/sql-server), Netdata needs access to internal server data. This requires configuration on both the Windows system and Netdata sides.

## Configuring

### Step 1: Configure Windows Defender Firewall

Netdata connects to SQL Server using TCP port 1433 (or your custom-configured port). You'll need to create a firewall rule to allow this connection:

1. Open `Windows Defender Firewall with Advanced Security` as an Administrator
2. Right-click `Inbound Rules` and select `New Rule...`
3. Select `Port`, then click `Next`
4. Choose `TCP`, enter `1433` in the `Specific local ports:` field, then click `Next`
5. Select an appropriate action (typically `Allow the connection`), then click `Next`
6. Choose where the rule applies (Domain, Private, or Public networks), then click `Next`
7. Provide a name and optional description for the rule, then click `Finish`

For multiple SQL Server instances, you can specify multiple ports in the same rule by separating them with commas.

### Step 2: Configure SQL Server Network Settings

Enable SQL Server to accept TCP connections:

1. Open `SQL Server Configuration Manager`
2. Expand `SQL Server Network Configuration`
3. Select `Protocols for <instance name>` in the console panel
4. Double-click the `TCP` protocol in the details panel and set `Enabled` to `Yes`
5. Go to the `IP Address` tab and locate the `IPAII` section:
    - Clear any value from the `TCP Dynamic Ports` field
    - Enter a port number in the `TCP Port` field (default is `1433`)
6. Select `SQL Server Services` and restart your SQL Server instance

These steps must be performed for each SQL Server instance you want to monitor.

### Step 3: Configure SQL Server Authentication

If you're using SQL Server authentication (rather than Windows authentication):

1. Open `SQL Server Management Studio`
2. Right-click your server and select `Properties`
3. Select `Security` in the left panel
4. Choose `SQL Server and Windows Authentication mode` under `Server authentication`
5. Click `OK`
6. Right-click your server and select `Restart`

### Step 4: Create a Monitoring User

Create an SQL Server user with the necessary permissions to collect monitoring data:

```tsql
USE master;
CREATE LOGIN netdata_user WITH PASSWORD = '1ReallyStrongPasswordShouldBeInsertedHere';
CREATE USER netdata_user FOR LOGIN netdata_user;
GRANT CONNECT SQL TO netdata_user;
GRANT VIEW SERVER STATE TO netdata_user;
GO
```

Additionally, enable the [Query Store](https://learn.microsoft.com/en-us/sql/relational-databases/performance/monitoring-performance-by-using-the-query-store?view=sql-server-ver16) on each database you want to monitor:

```tsql
DECLARE @dbname NVARCHAR(max)
DECLARE nd_user_cursor CURSOR FOR SELECT name
                                  FROM master.dbo.sysdatabases
                                  WHERE name NOT IN ('master', 'tempdb')

OPEN nd_user_cursor
FETCH NEXT FROM nd_user_cursor INTO @dbname
WHILE @@FETCH_STATUS = 0
    BEGIN
        EXECUTE ("USE "+ @dbname+"; CREATE USER netdata_user FOR LOGIN netdata_user; ALTER DATABASE "+@dbname+" SET QUERY_STORE = ON ( QUERY_CAPTURE_MODE = ALL, DATA_FLUSH_INTERVAL_SECONDS = 900 )");
        FETCH next FROM nd_user_cursor INTO @dbname;
    END
CLOSE nd_user_cursor
DEALLOCATE nd_user_cursor
GO
```

Apply this configuration to each SQL Server instance you wish to monitor.

### Step 5: Configure Netdata

Add the SQL Server connection details to your `netdata.conf` file:

```
[plugin:windows:PerflibMSSQL]
        driver = SQL Server
        server = 127.0.0.1\\Dev, 1433
        #address = [protocol:]Address[,port |\pipe\pipename]
        uid = netdata_user
        pwd = 1ReallyStrongPasswordShouldBeInsertedHere
        # additional instances = 0
        #windows authentication = no
```

Configuration options:

| Option                   | Description                                                                  |
|--------------------------|------------------------------------------------------------------------------|
| `driver`                 | ODBC driver used to connect to the SQL Server                                |
| `server`                 | Server address or instance name                                              |
| `address`                | Alternative to `server`; supports named pipes if the server supports them    |
| `uid`                    | SQL Server user identifier                                                   |
| `pwd`                    | Password for the specified user                                              |
| `additional instances`   | Number of additional SQL Server instances to monitor                         |
| `windows authentication` | Set to `yes` to use Windows credentials instead of SQL Server authentication |

For more information on connection parameters, see the [Microsoft Official Documentation](https://learn.microsoft.com/en-us/sql/relational-databases/native-client/applications/using-connection-string-keywords-with-sql-server-native-client?view=sql-server-ver15&viewFallbackFrom=sql-server-ver16).

## Monitoring Multiple SQL Server Instances

SQL Server can host multiple instances on the same machine. To monitor additional instances:

1. Update the main section to specify the number of additional instances:
   ```text
   [plugin:windows:PerflibMSSQL]
        additional instances = 1
   ```
2. Add a new configuration section for each additional instance, using sequential numbering:
   ```text
   [plugin:windows:PerflibMSSQL1]
        driver = SQL Server
        server = 127.0.0.1\\Production, 1434
        uid = netdata_user
        pwd = AnotherReallyStrongPasswordShouldBeInsertedHere2$
   ```

Each additional instance section must follow the naming pattern `plugin:windows:PerflibMSSQL` with a sequential number from 1 to 65535.

## Troubleshooting Common Issues

### Data source name is not found and no default driver

This error occurs when the specified ODBC driver is incorrect. To check available drivers:

1. Open `ODBC Data Sources`
2. Go to the `Drivers` tab
3. Look for the correct name of the `ODBC` or SQL `Server driver`

### Database Metrics Not Visible

If a database isn't appearing on the Netdata dashboard, statistics collection might not be enabled:

1. Open `SQL Server Configuration Manager`
2. Right-click the database and select `Properties`
3. Select `Options` in the left pane
4. Set `Auto Create Statistics` to `True`

### Login Failed

If authentication fails, check the SQL Server error log at:

```
C:\Program Files\Microsoft SQL Server\VERSION\MSSQL\Log\ERRORLOG
```

Where `VERSION` corresponds to your SQL Server version.

You can also check Windows Event Viewer for entries beginning with `MSSQL server error using the handle`.

