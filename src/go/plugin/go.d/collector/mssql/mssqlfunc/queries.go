// SPDX-License-Identifier: GPL-3.0-or-later

package mssqlfunc

// A wildcard or Azure blob prefix also matches other targets whose configured names
// share that prefix. Accept only this target's numeric rollover suffix to exclude those files.
const queryMSSQLXEventFileEvents = `
WITH file_events AS (
  SELECT
    CAST(event_data AS XML) AS event_xml,
    RIGHT(
      file_name,
      CHARINDEX('/', REVERSE('/' + REPLACE(file_name, '\', '/'))) - 1
    ) AS file_basename
  FROM sys.fn_xe_file_target_read_file(@filePath, NULL, NULL, NULL)
  WHERE object_name = @eventName
), xevents AS (
  SELECT event_xml
  FROM file_events
  WHERE LEN(file_basename) > LEN(@filePrefix) + 4
    AND LEFT(file_basename, LEN(@filePrefix)) = @filePrefix
    AND RIGHT(file_basename, 4) = '.xel'
    AND SUBSTRING(
      file_basename,
      LEN(@filePrefix) + 1,
      CASE
        WHEN LEN(file_basename) > LEN(@filePrefix) + 4
        THEN LEN(file_basename) - LEN(@filePrefix) - 4
        ELSE 0
      END
    ) NOT LIKE '%[^0123456789]%'
)
`

// querySystemHealthLatestDeadlockEventFile retrieves the latest xml_deadlock_report event
// from the system_health Extended Events file target.
const querySystemHealthLatestDeadlockEventFile = queryMSSQLXEventFileEvents + `
SELECT TOP (1)
  event_xml.value('(/event/@timestamp)[1]', 'datetime2(7)') AS deadlock_time,
  CONVERT(nvarchar(max), event_xml.query('(/event/data[@name="xml_report"]/value/deadlock)[1]')) AS deadlock_xml
FROM xevents
ORDER BY deadlock_time DESC;
`

// querySystemHealthLatestDeadlockRingBuffer retrieves the latest xml_deadlock_report event
// from the system_health Extended Events ring buffer.
// Note: This can be slow with large buffers due to XML parsing overhead.
const querySystemHealthLatestDeadlockRingBuffer = `
WITH xevents AS (
  SELECT CAST(xet.target_data AS XML) AS target_data
  FROM sys.dm_xe_session_targets AS xet
  JOIN sys.dm_xe_sessions AS xs ON xs.address = xet.event_session_address
  WHERE xs.name = 'system_health'
    AND xet.target_name = 'ring_buffer'
)
SELECT TOP (1)
  xevent.value('@timestamp', 'datetime2(7)') AS deadlock_time,
  CONVERT(nvarchar(max), xevent.query('(data/value/deadlock)[1]')) AS deadlock_xml
FROM xevents
CROSS APPLY target_data.nodes('RingBufferTarget/event[@name="xml_deadlock_report"]') AS T(xevent)
ORDER BY deadlock_time DESC;
`

// queryDatabaseNamesByID retrieves database_id to name mappings.
const queryDatabaseNamesByID = `
SELECT database_id, name
FROM sys.databases;
`

// queryQueryStoreSupported reports whether this instance exposes Query Store.
// sys.databases.is_query_store_on arrived in SQL Server 2016 (13.x). Probing for the
// column rather than comparing ProductVersion keeps Azure SQL Database working: it
// reports version 12.x yet is newer than SQL Server 2016 and does support Query Store.
const queryQueryStoreSupported = `
SELECT COUNT(*)
FROM sys.all_columns AS c
INNER JOIN sys.all_objects AS o ON o.object_id = c.object_id
WHERE o.name = 'databases'
  AND o.schema_id = SCHEMA_ID('sys')
  AND c.name = 'is_query_store_on';
`

// queryPlanCacheColumns discovers which sys.dm_exec_query_stats columns this release has.
// The DMV exists since SQL Server 2005 but gained columns over time, so top-queries probes
// it instead of gating on a version number.
const queryPlanCacheColumns = `SELECT TOP 0 * FROM sys.dm_exec_query_stats`

// queryMSSQLXEventSessionEventFilePath returns the filename configured on the session's
// event_file target. The on-disk name is operator-chosen and need not match the session
// name, so it has to be read from the catalog rather than guessed.
const queryMSSQLXEventSessionEventFilePath = `
SELECT CONVERT(nvarchar(260), fld.value) AS file_path
FROM sys.server_event_sessions AS ses
INNER JOIN sys.server_event_session_targets AS tgt
  ON tgt.event_session_id = ses.event_session_id
INNER JOIN sys.server_event_session_fields AS fld
  ON fld.event_session_id = tgt.event_session_id
 AND fld.object_id = tgt.target_id
WHERE ses.name = @sessionName
  AND tgt.name = 'event_file'
  AND fld.name = 'filename';
`

const queryMSSQLXEventDatabaseSessionEventFilePath = `
SELECT CONVERT(nvarchar(2048), fld.value) AS file_path
FROM sys.database_event_sessions AS ses
INNER JOIN sys.database_event_session_targets AS tgt
  ON tgt.event_session_id = ses.event_session_id
INNER JOIN sys.database_event_session_fields AS fld
  ON fld.event_session_id = tgt.event_session_id
 AND fld.object_id = tgt.target_id
WHERE ses.name = @sessionName
  AND tgt.name = 'event_file'
  AND fld.name = 'filename';
`

// queryMSSQLXEventSessionHasRingBuffer verifies that the session has a ring_buffer target.
const queryMSSQLXEventSessionHasRingBuffer = `
SELECT COUNT(*)
FROM sys.dm_xe_session_targets AS xet
JOIN sys.dm_xe_sessions AS xs ON xs.address = xet.event_session_address
WHERE xs.name = @sessionName
  AND xet.target_name = 'ring_buffer';
`

const queryMSSQLXEventDatabaseSessionHasRingBuffer = `
SELECT COUNT(*)
FROM sys.dm_xe_database_session_targets AS xet
JOIN sys.dm_xe_database_sessions AS xs ON xs.address = xet.event_session_address
WHERE xs.name = @sessionName
  AND xet.target_name = 'ring_buffer';
`

// queryMSSQLErrorInfoEventFile reads recent error_reported events from the event_file target.
// @filePath is the resolved on-disk pattern, not the session name; see
// queryMSSQLXEventSessionEventFilePath.
//
// query_hash is returned as the raw unsigned-64-bit decimal string that Extended Events
// emits. It is not converted here because query_hash exceeds bigint range, so
// CAST(... AS bigint) would overflow; mssqlQueryHashToHex does it in Go instead.
const queryMSSQLErrorInfoEventFile = queryMSSQLXEventFileEvents + `
SELECT TOP (@limit)
  event_xml.value('(/event/@timestamp)[1]', 'datetime2(7)') AS event_time,
  event_xml.value('(/event/data[@name="error_number"]/value)[1]', 'int') AS error_number,
  event_xml.value('(/event/data[@name="state"]/value)[1]', 'int') AS error_state,
  event_xml.value('(/event/data[@name="message"]/value)[1]', 'nvarchar(max)') AS message,
  event_xml.value('(/event/action[@name="sql_text"]/value)[1]', 'nvarchar(max)') AS sql_text,
  event_xml.value('(/event/action[@name="query_hash"]/value)[1]', 'nvarchar(32)') AS query_hash
FROM xevents
ORDER BY event_time DESC;
`

// queryMSSQLErrorInfoRingBuffer reads recent error_reported events from the ring_buffer target.
// Note: This can be slow with large buffers due to XML parsing overhead.
// query_hash is returned raw for the same reason as queryMSSQLErrorInfoEventFile.
const queryMSSQLErrorInfoRingBuffer = `
WITH xevents AS (
  SELECT CAST(xet.target_data AS XML) AS target_data
  FROM sys.dm_xe_session_targets AS xet
  JOIN sys.dm_xe_sessions AS xs ON xs.address = xet.event_session_address
  WHERE xs.name = @sessionName
    AND xet.target_name = 'ring_buffer'
)
SELECT TOP (@limit)
  xevent.value('@timestamp', 'datetime2(7)') AS event_time,
  xevent.value('(data[@name="error_number"]/value)[1]', 'int') AS error_number,
  xevent.value('(data[@name="state"]/value)[1]', 'int') AS error_state,
  xevent.value('(data[@name="message"]/value)[1]', 'nvarchar(max)') AS message,
  xevent.value('(action[@name="sql_text"]/value)[1]', 'nvarchar(max)') AS sql_text,
  xevent.value('(action[@name="query_hash"]/value)[1]', 'nvarchar(32)') AS query_hash
FROM xevents
CROSS APPLY target_data.nodes('RingBufferTarget/event[@name="error_reported"]') AS T(xevent)
ORDER BY event_time DESC;
`

const queryMSSQLErrorInfoDatabaseRingBuffer = `
WITH xevents AS (
  SELECT CAST(xet.target_data AS XML) AS target_data
  FROM sys.dm_xe_database_session_targets AS xet
  JOIN sys.dm_xe_database_sessions AS xs ON xs.address = xet.event_session_address
  WHERE xs.name = @sessionName
    AND xet.target_name = 'ring_buffer'
)
SELECT TOP (@limit)
  xevent.value('@timestamp', 'datetime2(7)') AS event_time,
  xevent.value('(data[@name="error_number"]/value)[1]', 'int') AS error_number,
  xevent.value('(data[@name="state"]/value)[1]', 'int') AS error_state,
  xevent.value('(data[@name="message"]/value)[1]', 'nvarchar(max)') AS message,
  xevent.value('(action[@name="sql_text"]/value)[1]', 'nvarchar(max)') AS sql_text,
  xevent.value('(action[@name="query_hash"]/value)[1]', 'nvarchar(32)') AS query_hash
FROM xevents
CROSS APPLY target_data.nodes('RingBufferTarget/event[@name="error_reported"]') AS T(xevent)
ORDER BY event_time DESC;
`
