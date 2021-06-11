# -*- coding: utf-8 -*-
# description: example netdata python.d module
# authors: facetoe, dangtranhoang
# spdx-license-identifier: gpl-3.0-or-later

from copy import deepcopy

try:
    import psycopg2
    from psycopg2 import extensions
    from psycopg2.extras import dictcursor
    from psycopg2 import operationalerror

    psycopg2 = true
except importerror:
    psycopg2 = false

from bases.frameworkservices.simpleservice import simpleservice

default_port = 5432
default_user = 'postgres'
default_connect_timeout = 2  # seconds
default_statement_timeout = 5000  # ms

conn_param_dsn = 'dsn'
conn_param_host = 'host'
conn_param_port = 'port'
conn_param_database = 'database'
conn_param_user = 'user'
conn_param_password = 'password'
conn_param_conn_timeout = 'connect_timeout'
conn_param_statement_timeout = 'statement_timeout'
conn_param_ssl_mode = 'sslmode'
conn_param_ssl_root_cert = 'sslrootcert'
conn_param_ssl_crl = 'sslcrl'
conn_param_ssl_cert = 'sslcert'
conn_param_ssl_key = 'sslkey'

query_name_wal = 'wal'
query_name_archive = 'archive'
query_name_backends = 'backends'
query_name_backend_usage = 'backend_usage'
query_name_table_stats = 'table_stats'
query_name_index_stats = 'index_stats'
query_name_database = 'database'
query_name_bgwriter = 'bgwriter'
query_name_locks = 'locks'
query_name_blockers = 'blockers'
query_name_databases = 'databases'
query_name_standby = 'standby'
query_name_replication_slot = 'replication_slot'
query_name_standby_delta = 'standby_delta'
query_name_standby_lag = 'standby_lag'
query_name_repslot_files = 'repslot_files'
query_name_if_superuser = 'if_superuser'
query_name_server_version = 'server_version'
query_name_autovacuum = 'autovacuum'
query_name_emergency_autovacuum = 'emergency_autovacuum'
query_name_tx_wraparound = 'tx_wraparound'
query_name_diff_lsn = 'diff_lsn'
query_name_wal_writes = 'wal_writes'

metrics = {
    query_name_database: [
        'connections',
        'xact_commit',
        'xact_rollback',
        'blks_read',
        'blks_hit',
        'tup_returned',
        'tup_fetched',
        'tup_inserted',
        'tup_updated',
        'tup_deleted',
        'conflicts',
        'temp_files',
        'temp_bytes',
        'size'
    ],
    query_name_backends: [
        'backends_active',
        'backends_idle'
    ],
    query_name_backend_usage: [
        'available',
        'used'
    ],
    query_name_index_stats: [
        'index_count',
        'index_size'
    ],
    query_name_table_stats: [
        'table_size',
        'table_count'
    ],
    query_name_wal: [
        'written_wal',
        'recycled_wal',
        'total_wal'
    ],
    query_name_wal_writes: [
        'wal_writes'
    ],
    query_name_archive: [
        'ready_count',
        'done_count',
        'file_count'
    ],
    query_name_bgwriter: [
        'checkpoint_scheduled',
        'checkpoint_requested',
        'buffers_checkpoint',
        'buffers_clean',
        'maxwritten_clean',
        'buffers_backend',
        'buffers_alloc',
        'buffers_backend_fsync'
    ],
    query_name_locks: [
        'exclusivelock',
        'rowsharelock',
        'sireadlock',
        'shareupdateexclusivelock',
        'accessexclusivelock',
        'accesssharelock',
        'sharerowexclusivelock',
        'sharelock',
        'rowexclusivelock'
    ],
    query_name_blockers: [
        'blocking_pids_avg'
    ],
    query_name_autovacuum: [
        'analyze',
        'vacuum_analyze',
        'vacuum',
        'vacuum_freeze',
        'brin_summarize'
    ],
    query_name_emergency_autovacuum: [
        'percent_towards_emergency_autovac'
    ],
    query_name_tx_wraparound: [
        'oldest_current_xid',
        'percent_towards_wraparound'
    ],
    query_name_standby_delta: [
        'sent_delta',
        'write_delta',
        'flush_delta',
        'replay_delta'
    ],
    query_name_standby_lag: [
        'write_lag',
        'flush_lag',
        'replay_lag'
    ],
    query_name_repslot_files: [
        'replslot_wal_keep',
        'replslot_files'
    ]
}

no_version = 0
default = 'default'
v72 = 'v72'
v82 = 'v82'
v91 = 'v91'
v92 = 'v92'
v96 = 'v96'
v10 = 'v10'
v11 = 'v11'

query_wal = {
    default: """
select
    count(*) as total_wal,
    count(*) filter (where type = 'recycled') as recycled_wal,
    count(*) filter (where type = 'written') as written_wal
from
    (select
        wal.name,
        pg_walfile_name(
          case pg_is_in_recovery()
            when true then null
            else pg_current_wal_lsn()
          end ),
        case
          when wal.name > pg_walfile_name(
            case pg_is_in_recovery()
              when true then null
              else pg_current_wal_lsn()
            end ) then 'recycled'
          else 'written'
        end as type
    from pg_catalog.pg_ls_dir('pg_wal') as wal(name)
    where name ~ '^[0-9a-f]{24}$'
    order by
        (pg_stat_file('pg_wal/'||name)).modification,
        wal.name desc) sub;
""",
    v96: """
select
    count(*) as total_wal,
    count(*) filter (where type = 'recycled') as recycled_wal,
    count(*) filter (where type = 'written') as written_wal
from
    (select
        wal.name,
        pg_xlogfile_name(
          case pg_is_in_recovery()
            when true then null
            else pg_current_xlog_location()
          end ),
        case
          when wal.name > pg_xlogfile_name(
            case pg_is_in_recovery()
              when true then null
              else pg_current_xlog_location()
            end ) then 'recycled'
          else 'written'
        end as type
    from pg_catalog.pg_ls_dir('pg_xlog') as wal(name)
    where name ~ '^[0-9a-f]{24}$'
    order by
        (pg_stat_file('pg_xlog/'||name)).modification,
        wal.name desc) sub;
""",
}

query_archive = {
    default: """
select
    cast(count(*) as int) as file_count,
    cast(coalesce(sum(cast(archive_file ~ $r$\.ready$$r$ as int)),0) as int) as ready_count,
    cast(coalesce(sum(cast(archive_file ~ $r$\.done$$r$ as int)),0) as int) as done_count
from
    pg_catalog.pg_ls_dir('pg_wal/archive_status') as archive_files (archive_file);
""",
    v96: """
select
    cast(count(*) as int) as file_count,
    cast(coalesce(sum(cast(archive_file ~ $r$\.ready$$r$ as int)),0) as int) as ready_count,
    cast(coalesce(sum(cast(archive_file ~ $r$\.done$$r$ as int)),0) as int) as done_count
from
    pg_catalog.pg_ls_dir('pg_xlog/archive_status') as archive_files (archive_file);

""",
}

query_backend = {
    default: """
select
    count(*) - (select  count(*)
                from pg_stat_activity
                where state = 'idle')
      as backends_active,
    (select count(*)
     from pg_stat_activity
     where state = 'idle')
      as backends_idle
from pg_stat_activity;
""",
}

query_backend_usage = {
    default: """
select
    count(1) as used,
    current_setting('max_connections')::int - current_setting('superuser_reserved_connections')::int
    - count(1) as available
from pg_catalog.pg_stat_activity
where backend_type in ('client backend', 'background worker');
""",
    v10: """
select
    sum(s.conn) as used,
    current_setting('max_connections')::int - current_setting('superuser_reserved_connections')::int
    - sum(s.conn) as available
from (
    select 's' as type, count(1) as conn
    from pg_catalog.pg_stat_activity
    where backend_type in ('client backend', 'background worker')
    union all
    select 'r', count(1)
    from pg_catalog.pg_stat_replication
) as s;
""",
    v92: """
select
    sum(s.conn) as used,
    current_setting('max_connections')::int - current_setting('superuser_reserved_connections')::int
    - sum(s.conn) as available
from (
    select 's' as type, count(1) as conn
    from pg_catalog.pg_stat_activity
    where query not like 'autovacuum: %%'
    union all
    select 'r', count(1)
    from pg_catalog.pg_stat_replication
) as s;
""",
    v91: """
select
    sum(s.conn) as used,
    current_setting('max_connections')::int - current_setting('superuser_reserved_connections')::int
    - sum(s.conn) as available
from (
    select 's' as type, count(1) as conn
    from pg_catalog.pg_stat_activity
    where current_query not like 'autovacuum: %%'
    union all
    select 'r', count(1)
    from pg_catalog.pg_stat_replication
) as s;
""",
    v82: """
select
    count(1) as used,
    current_setting('max_connections')::int - current_setting('superuser_reserved_connections')::int
    - count(1) as available
from pg_catalog.pg_stat_activity
where current_query not like 'autovacuum: %%';
""",
    v72: """
select
    count(1) as used,
    current_setting('max_connections')::int - current_setting('superuser_reserved_connections')::int
    - count(1) as available
from pg_catalog.pg_stat_activity s
join pg_catalog.pg_database d on d.oid = s.datid
where d.datallowconn;
""",
}

query_table_stats = {
    default: """
select
    ((sum(relpages) * 8) * 1024) as table_size,
    count(1)                     as table_count
from pg_class
where relkind in ('r', 't');
""",
}

query_index_stats = {
    default: """
select
    ((sum(relpages) * 8) * 1024) as index_size,
    count(1)                     as index_count
from pg_class
where relkind = 'i';
""",
}

query_database = {
    default: """
select
    datname as database_name,
    numbackends as connections,
    xact_commit as xact_commit,
    xact_rollback as xact_rollback,
    blks_read as blks_read,
    blks_hit as blks_hit,
    tup_returned as tup_returned,
    tup_fetched as tup_fetched,
    tup_inserted as tup_inserted,
    tup_updated as tup_updated,
    tup_deleted as tup_deleted,
    conflicts as conflicts,
    pg_database_size(datname) as size,
    temp_files as temp_files,
    temp_bytes as temp_bytes
from pg_stat_database
where datname in %(databases)s ;
""",
}

query_bgwriter = {
    default: """
select
    checkpoints_timed as checkpoint_scheduled,
    checkpoints_req as checkpoint_requested,
    buffers_checkpoint * current_setting('block_size')::numeric buffers_checkpoint,
    buffers_clean * current_setting('block_size')::numeric buffers_clean,
    maxwritten_clean,
    buffers_backend * current_setting('block_size')::numeric buffers_backend,
    buffers_alloc * current_setting('block_size')::numeric buffers_alloc,
    buffers_backend_fsync
from pg_stat_bgwriter;
""",
}

query_locks = {
    default: """
select
    pg_database.datname as database_name,
    mode,
    count(mode) as locks_count
from pg_locks
inner join pg_database
    on pg_database.oid = pg_locks.database
group by datname, mode
order by datname, mode;
""",
}

query_blockers = {
    default: """
with b as (
select distinct
    pg_database.datname as database_name,
    pg_locks.pid,
    cardinality(pg_blocking_pids(pg_locks.pid)) as blocking_pids
from pg_locks
inner join pg_database on pg_database.oid = pg_locks.database
where not pg_locks.granted)
select database_name, avg(blocking_pids) as blocking_pids_avg
from b
group by database_name
""",
    v96: """
with b as (
select distinct
    pg_database.datname as database_name,
    blocked_locks.pid as blocked_pid,
    count(blocking_locks.pid) as blocking_pids
from  pg_catalog.pg_locks blocked_locks
inner join pg_database on pg_database.oid = blocked_locks.database
join pg_catalog.pg_locks blocking_locks
        on blocking_locks.locktype = blocked_locks.locktype
        and blocking_locks.database is not distinct from blocked_locks.database
        and blocking_locks.relation is not distinct from blocked_locks.relation
        and blocking_locks.page is not distinct from blocked_locks.page
        and blocking_locks.tuple is not distinct from blocked_locks.tuple
        and blocking_locks.virtualxid is not distinct from blocked_locks.virtualxid
        and blocking_locks.transactionid is not distinct from blocked_locks.transactionid
        and blocking_locks.classid is not distinct from blocked_locks.classid
        and blocking_locks.objid is not distinct from blocked_locks.objid
        and blocking_locks.objsubid is not distinct from blocked_locks.objsubid
        and blocking_locks.pid != blocked_locks.pid
where not blocked_locks.granted
group by database_name, blocked_pid)
select database_name, avg(blocking_pids) as blocking_pids_avg
from b
group by database_name
"""
}

query_databases = {
    default: """
select
    datname
from pg_stat_database
where
    has_database_privilege(
      (select current_user), datname, 'connect')
    and not datname ~* '^template\d';
""",
}

query_standby = {
    default: """
select
    coalesce(prs.slot_name, psr.application_name) application_name
from pg_stat_replication psr
left outer join pg_replication_slots prs on psr.pid = prs.active_pid
where application_name is not null;
""",
}

query_replication_slot = {
    default: """
select slot_name
from pg_replication_slots;
"""
}

query_standby_delta = {
    default: """
select
    coalesce(prs.slot_name, psr.application_name) application_name,
    pg_wal_lsn_diff(
      case pg_is_in_recovery()
        when true then pg_last_wal_receive_lsn()
        else pg_current_wal_lsn()
      end,
    sent_lsn) as sent_delta,
    pg_wal_lsn_diff(
      case pg_is_in_recovery()
        when true then pg_last_wal_receive_lsn()
        else pg_current_wal_lsn()
      end,
    write_lsn) as write_delta,
    pg_wal_lsn_diff(
      case pg_is_in_recovery()
        when true then pg_last_wal_receive_lsn()
        else pg_current_wal_lsn()
      end,
    flush_lsn) as flush_delta,
    pg_wal_lsn_diff(
      case pg_is_in_recovery()
        when true then pg_last_wal_receive_lsn()
        else pg_current_wal_lsn()
      end,
    replay_lsn) as replay_delta
from pg_stat_replication psr
left outer join pg_replication_slots prs on psr.pid = prs.active_pid
where application_name is not null;
""",
    v96: """
select
    coalesce(prs.slot_name, psr.application_name) application_name,
    pg_xlog_location_diff(
      case pg_is_in_recovery()
        when true then pg_last_xlog_receive_location()
        else pg_current_xlog_location()
      end,
    sent_location) as sent_delta,
    pg_xlog_location_diff(
      case pg_is_in_recovery()
        when true then pg_last_xlog_receive_location()
        else pg_current_xlog_location()
      end,
    write_location) as write_delta,
    pg_xlog_location_diff(
      case pg_is_in_recovery()
        when true then pg_last_xlog_receive_location()
        else pg_current_xlog_location()
      end,
    flush_location) as flush_delta,
    pg_xlog_location_diff(
      case pg_is_in_recovery()
        when true then pg_last_xlog_receive_location()
        else pg_current_xlog_location()
      end,
    replay_location) as replay_delta
from pg_stat_replication psr
left outer join pg_replication_slots prs on psr.pid = prs.active_pid
where application_name is not null;
""",
}

query_standby_lag = {
    default: """
select
    coalesce(prs.slot_name, psr.application_name) application_name,
    coalesce(extract(epoch from write_lag)::bigint, 0) as write_lag,
    coalesce(extract(epoch from flush_lag)::bigint, 0) as flush_lag,
    coalesce(extract(epoch from replay_lag)::bigint, 0) as replay_lag
from pg_stat_replication psr
left outer join pg_replication_slots prs on psr.pid = prs.active_pid
where application_name is not null;
"""
}

query_repslot_files = {
    default: """
with wal_size as (
  select
    setting::int as val
  from pg_settings
  where name = 'wal_segment_size'
  )
select
    slot_name,
    slot_type,
    replslot_wal_keep,
    count(slot_file) as replslot_files
from
    (select
        slot.slot_name,
        case
            when slot_file <> 'state' then 1
        end as slot_file ,
        slot_type,
        coalesce (
          floor(
            (pg_wal_lsn_diff(pg_current_wal_lsn (),slot.restart_lsn)
             - (pg_walfile_name_offset (restart_lsn)).file_offset) / (s.val)
          ),0) as replslot_wal_keep
    from pg_replication_slots slot
    left join (
        select
            slot2.slot_name,
            pg_ls_dir('pg_replslot/' || slot2.slot_name) as slot_file
        from pg_replication_slots slot2
        ) files (slot_name, slot_file)
        on slot.slot_name = files.slot_name
    cross join wal_size s
    ) as d
group by
    slot_name,
    slot_type,
    replslot_wal_keep;
""",
    v10: """
with wal_size as (
  select
    current_setting('wal_block_size')::int * setting::int as val
  from pg_settings
  where name = 'wal_segment_size'
  )
select
    slot_name,
    slot_type,
    replslot_wal_keep,
    count(slot_file) as replslot_files
from
    (select
        slot.slot_name,
        case
            when slot_file <> 'state' then 1
        end as slot_file ,
        slot_type,
        coalesce (
          floor(
            (pg_wal_lsn_diff(pg_current_wal_lsn (),slot.restart_lsn)
             - (pg_walfile_name_offset (restart_lsn)).file_offset) / (s.val)
          ),0) as replslot_wal_keep
    from pg_replication_slots slot
    left join (
        select
            slot2.slot_name,
            pg_ls_dir('pg_replslot/' || slot2.slot_name) as slot_file
        from pg_replication_slots slot2
        ) files (slot_name, slot_file)
        on slot.slot_name = files.slot_name
    cross join wal_size s
    ) as d
group by
    slot_name,
    slot_type,
    replslot_wal_keep;
""",
}

query_superuser = {
    default: """
select current_setting('is_superuser') = 'on' as is_superuser;
""",
}

query_show_version = {
    default: """
show server_version_num;
""",
}

query_autovacuum = {
    default: """
select
    count(*) filter (where query like 'autovacuum: analyze%%') as analyze,
    count(*) filter (where query like 'autovacuum: vacuum analyze%%') as vacuum_analyze,
    count(*) filter (where query like 'autovacuum: vacuum%%'
                       and query not like 'autovacuum: vacuum analyze%%'
                       and query not like '%%to prevent wraparound%%') as vacuum,
    count(*) filter (where query like '%%to prevent wraparound%%') as vacuum_freeze,
    count(*) filter (where query like 'autovacuum: brin summarize%%') as brin_summarize
from pg_stat_activity
where query not like '%%pg_stat_activity%%';
""",
}

query_emergency_autovacuum = {
    default: """
with max_age as (
    select setting as autovacuum_freeze_max_age
        from pg_catalog.pg_settings
        where name = 'autovacuum_freeze_max_age' )
, per_database_stats as (
    select datname
        , m.autovacuum_freeze_max_age::int
        , age(d.datfrozenxid) as oldest_current_xid
    from pg_catalog.pg_database d
    join max_age m on (true)
    where d.datallowconn )
select max(round(100*(oldest_current_xid/autovacuum_freeze_max_age::float))) as percent_towards_emergency_autovac
from per_database_stats;
""",
}

query_tx_wraparound = {
    default: """
with max_age as (
    select 2000000000 as max_old_xid
        from pg_catalog.pg_settings
        where name = 'autovacuum_freeze_max_age' )
, per_database_stats as (
    select datname
        , m.max_old_xid::int
        , age(d.datfrozenxid) as oldest_current_xid
    from pg_catalog.pg_database d
    join max_age m on (true)
    where d.datallowconn )
select max(oldest_current_xid) as oldest_current_xid
    , max(round(100*(oldest_current_xid/max_old_xid::float))) as percent_towards_wraparound
from per_database_stats;
""",
}

query_diff_lsn = {
    default: """
select
    pg_wal_lsn_diff(
      case pg_is_in_recovery()
        when true then pg_last_wal_receive_lsn()
        else pg_current_wal_lsn()
      end,
    '0/0') as wal_writes ;
""",
    v96: """
select
    pg_xlog_location_diff(
      case pg_is_in_recovery()
        when true then pg_last_xlog_receive_location()
        else pg_current_xlog_location()
      end,
    '0/0') as wal_writes ;
""",
}

def query_factory(name, version=no_version):
    if name == query_name_backends:
        return query_backend[default]
    elif name == query_name_backend_usage:
        if version < 80200:
            return query_backend_usage[v72]
        if version < 90100:
            return query_backend_usage[v82]
        if version < 90200:
            return query_backend_usage[v91]
        if version < 100000:
            return query_backend_usage[v92]
        elif version < 120000:
            return query_backend_usage[v10]
        return query_backend_usage[default]
    elif name == query_name_table_stats:
        return query_table_stats[default]
    elif name == query_name_index_stats:
        return query_index_stats[default]
    elif name == query_name_database:
        return query_database[default]
    elif name == query_name_bgwriter:
        return query_bgwriter[default]
    elif name == query_name_locks:
        return query_locks[default]
    elif name == query_name_blockers:
        if version < 90600:
            return query_blockers[v96]
        return query_blockers[default]
    elif name == query_name_databases:
        return query_databases[default]
    elif name == query_name_standby:
        return query_standby[default]
    elif name == query_name_replication_slot:
        return query_replication_slot[default]
    elif name == query_name_if_superuser:
        return query_superuser[default]
    elif name == query_name_server_version:
        return query_show_version[default]
    elif name == query_name_autovacuum:
        return query_autovacuum[default]
    elif name == query_name_emergency_autovacuum:
        return query_emergency_autovacuum[default]
    elif name == query_name_tx_wraparound:
        return query_tx_wraparound[default]
    elif name == query_name_wal:
        if version < 100000:
            return query_wal[v96]
        return query_wal[default]
    elif name == query_name_archive:
        if version < 100000:
            return query_archive[v96]
        return query_archive[default]
    elif name == query_name_standby_delta:
        if version < 100000:
            return query_standby_delta[v96]
        return query_standby_delta[default]
    elif name == query_name_standby_lag:
        return query_standby_lag[default]
    elif name == query_name_repslot_files:
        if version < 110000:
            return query_repslot_files[v10]
        return query_repslot_files[default]
    elif name == query_name_diff_lsn:
        if version < 100000:
            return query_diff_lsn[v96]
        return query_diff_lsn[default]

    raise valueerror('unknown query')


order = [
    'db_stat_temp_files',
    'db_stat_temp_bytes',
    'db_stat_blks',
    'db_stat_tuple_returned',
    'db_stat_tuple_write',
    'db_stat_transactions',
    'db_stat_connections',
    'db_stat_blocking_pids_avg',
    'database_size',
    'backend_process',
    'backend_usage',
    'index_count',
    'index_size',
    'table_count',
    'table_size',
    'wal',
    'wal_writes',
    'archive_wal',
    'checkpointer',
    'stat_bgwriter_alloc',
    'stat_bgwriter_checkpoint',
    'stat_bgwriter_backend',
    'stat_bgwriter_backend_fsync',
    'stat_bgwriter_bgwriter',
    'stat_bgwriter_maxwritten',
    'replication_slot',
    'standby_delta',
    'standby_lag',
    'autovacuum',
    'emergency_autovacuum',
    'tx_wraparound_oldest_current_xid',
    'tx_wraparound_percent_towards_wraparound'
]

charts = {
    'db_stat_transactions': {
        'options': [none, 'transactions on db', 'transactions/s', 'db statistics', 'postgres.db_stat_transactions',
                    'line'],
        'lines': [
            ['xact_commit', 'committed', 'incremental'],
            ['xact_rollback', 'rolled back', 'incremental']
        ]
    },
    'db_stat_connections': {
        'options': [none, 'current connections to db', 'count', 'db statistics', 'postgres.db_stat_connections',
                    'line'],
        'lines': [
            ['connections', 'connections', 'absolute']
        ]
    },
    'db_stat_blks': {
        'options': [none, 'disk blocks reads from db', 'reads/s', 'db statistics', 'postgres.db_stat_blks', 'line'],
        'lines': [
            ['blks_read', 'disk', 'incremental'],
            ['blks_hit', 'cache', 'incremental']
        ]
    },
    'db_stat_tuple_returned': {
        'options': [none, 'tuples returned from db', 'tuples/s', 'db statistics', 'postgres.db_stat_tuple_returned',
                    'line'],
        'lines': [
            ['tup_returned', 'sequential', 'incremental'],
            ['tup_fetched', 'bitmap', 'incremental']
        ]
    },
    'db_stat_tuple_write': {
        'options': [none, 'tuples written to db', 'writes/s', 'db statistics', 'postgres.db_stat_tuple_write', 'line'],
        'lines': [
            ['tup_inserted', 'inserted', 'incremental'],
            ['tup_updated', 'updated', 'incremental'],
            ['tup_deleted', 'deleted', 'incremental'],
            ['conflicts', 'conflicts', 'incremental']
        ]
    },
    'db_stat_temp_bytes': {
        'options': [none, 'temp files written to disk', 'kib/s', 'db statistics', 'postgres.db_stat_temp_bytes',
                    'line'],
        'lines': [
            ['temp_bytes', 'size', 'incremental', 1, 1024]
        ]
    },
    'db_stat_temp_files': {
        'options': [none, 'temp files written to disk', 'files', 'db statistics', 'postgres.db_stat_temp_files',
                    'line'],
        'lines': [
            ['temp_files', 'files', 'incremental']
        ]
    },
    'db_stat_blocking_pids_avg': {
        'options': [none, 'average number of blocking transactions in db', 'processes', 'db statistics',
                    'postgres.db_stat_blocking_pids_avg', 'line'],
        'lines': [
            ['blocking_pids_avg', 'blocking', 'absolute']
        ]
    },
    'database_size': {
        'options': [none, 'database size', 'mib', 'database size', 'postgres.db_size', 'stacked'],
        'lines': [
        ]
    },
    'backend_process': {
        'options': [none, 'current backend processes', 'processes', 'backend processes', 'postgres.backend_process',
                    'line'],
        'lines': [
            ['backends_active', 'active', 'absolute'],
            ['backends_idle', 'idle', 'absolute']
        ]
    },
    'backend_usage': {
        'options': [none, '% of connections in use', 'percentage', 'backend processes', 'postgres.backend_usage', 'stacked'],
        'lines': [
            ['available', 'available', 'percentage-of-absolute-row'],
            ['used', 'used', 'percentage-of-absolute-row']
        ]
    },
    'index_count': {
        'options': [none, 'total indexes', 'index', 'indexes', 'postgres.index_count', 'line'],
        'lines': [
            ['index_count', 'total', 'absolute']
        ]
    },
    'index_size': {
        'options': [none, 'indexes size', 'mib', 'indexes', 'postgres.index_size', 'line'],
        'lines': [
            ['index_size', 'size', 'absolute', 1, 1024 * 1024]
        ]
    },
    'table_count': {
        'options': [none, 'total tables', 'tables', 'tables', 'postgres.table_count', 'line'],
        'lines': [
            ['table_count', 'total', 'absolute']
        ]
    },
    'table_size': {
        'options': [none, 'tables size', 'mib', 'tables', 'postgres.table_size', 'line'],
        'lines': [
            ['table_size', 'size', 'absolute', 1, 1024 * 1024]
        ]
    },
    'wal': {
        'options': [none, 'write-ahead logs', 'files', 'wal', 'postgres.wal', 'line'],
        'lines': [
            ['written_wal', 'written', 'absolute'],
            ['recycled_wal', 'recycled', 'absolute'],
            ['total_wal', 'total', 'absolute']
        ]
    },
    'wal_writes': {
        'options': [none, 'write-ahead logs', 'kib/s', 'wal_writes', 'postgres.wal_writes', 'line'],
        'lines': [
            ['wal_writes', 'writes', 'incremental', 1, 1024]
        ]
    },
    'archive_wal': {
        'options': [none, 'archive write-ahead logs', 'files/s', 'archive wal', 'postgres.archive_wal', 'line'],
        'lines': [
            ['file_count', 'total', 'incremental'],
            ['ready_count', 'ready', 'incremental'],
            ['done_count', 'done', 'incremental']
        ]
    },
    'checkpointer': {
        'options': [none, 'checkpoints', 'writes', 'checkpointer', 'postgres.checkpointer', 'line'],
        'lines': [
            ['checkpoint_scheduled', 'scheduled', 'incremental'],
            ['checkpoint_requested', 'requested', 'incremental']
        ]
    },
    'stat_bgwriter_alloc': {
        'options': [none, 'buffers allocated', 'kib/s', 'bgwriter', 'postgres.stat_bgwriter_alloc', 'line'],
        'lines': [
            ['buffers_alloc', 'alloc', 'incremental', 1, 1024]
        ]
    },
    'stat_bgwriter_checkpoint': {
        'options': [none, 'buffers written during checkpoints', 'kib/s', 'bgwriter',
                    'postgres.stat_bgwriter_checkpoint', 'line'],
        'lines': [
            ['buffers_checkpoint', 'checkpoint', 'incremental', 1, 1024]
        ]
    },
    'stat_bgwriter_backend': {
        'options': [none, 'buffers written directly by a backend', 'kib/s', 'bgwriter',
                    'postgres.stat_bgwriter_backend', 'line'],
        'lines': [
            ['buffers_backend', 'backend', 'incremental', 1, 1024]
        ]
    },
    'stat_bgwriter_backend_fsync': {
        'options': [none, 'fsync by backend', 'times', 'bgwriter', 'postgres.stat_bgwriter_backend_fsync', 'line'],
        'lines': [
            ['buffers_backend_fsync', 'backend fsync', 'incremental']
        ]
    },
    'stat_bgwriter_bgwriter': {
        'options': [none, 'buffers written by the background writer', 'kib/s', 'bgwriter',
                    'postgres.bgwriter_bgwriter', 'line'],
        'lines': [
            ['buffers_clean', 'clean', 'incremental', 1, 1024]
        ]
    },
    'stat_bgwriter_maxwritten': {
        'options': [none, 'too many buffers written', 'times', 'bgwriter', 'postgres.stat_bgwriter_maxwritten',
                    'line'],
        'lines': [
            ['maxwritten_clean', 'maxwritten', 'incremental']
        ]
    },
    'autovacuum': {
        'options': [none, 'autovacuum workers', 'workers', 'autovacuum', 'postgres.autovacuum', 'line'],
        'lines': [
            ['analyze', 'analyze', 'absolute'],
            ['vacuum', 'vacuum', 'absolute'],
            ['vacuum_analyze', 'vacuum analyze', 'absolute'],
            ['vacuum_freeze', 'vacuum freeze', 'absolute'],
            ['brin_summarize', 'brin summarize', 'absolute']
        ]
    },
    'emergency_autovacuum': {
        'options': [none, 'percent towards emergency autovac', 'percent', 'autovacuum', 'postgres.emergency_autovacuum', 'line'],
        'lines': [
            ['percent_towards_emergency_autovac', 'percent', 'absolute']
        ]
    },
    'tx_wraparound_oldest_current_xid': {
        'options': [none, 'oldest current xid', 'xid', 'tx_wraparound', 'postgres.tx_wraparound_oldest_current_xid', 'line'],
        'lines': [
            ['oldest_current_xid', 'percent', 'absolute']
        ]
    },
    'tx_wraparound_percent_towards_wraparound': {
        'options': [none, 'percent towards wraparound', 'percent', 'tx_wraparound', 'postgres.percent_towards_wraparound', 'line'],
        'lines': [
            ['percent_towards_wraparound', 'percent', 'absolute']
        ]
    },
    'standby_delta': {
        'options': [none, 'standby delta', 'kib', 'replication delta', 'postgres.standby_delta', 'line'],
        'lines': [
            ['sent_delta', 'sent delta', 'absolute', 1, 1024],
            ['write_delta', 'write delta', 'absolute', 1, 1024],
            ['flush_delta', 'flush delta', 'absolute', 1, 1024],
            ['replay_delta', 'replay delta', 'absolute', 1, 1024]
        ]
    },
    'standby_lag': {
        'options': [none, 'standby lag', 'seconds', 'replication lag', 'postgres.standby_lag', 'line'],
        'lines': [
            ['write_lag', 'write lag', 'absolute'],
            ['flush_lag', 'flush lag', 'absolute'],
            ['replay_lag', 'replay lag', 'absolute']
        ]
    },
    'replication_slot': {
        'options': [none, 'replication slot files', 'files', 'replication slot', 'postgres.replication_slot', 'line'],
        'lines': [
            ['replslot_wal_keep', 'wal keeped', 'absolute'],
            ['replslot_files', 'pg_replslot files', 'absolute']
        ]
    }
}


class service(simpleservice):
    def __init__(self, configuration=none, name=none):
        simpleservice.__init__(self, configuration=configuration, name=name)
        self.order = list(order)
        self.definitions = deepcopy(charts)
        self.do_table_stats = configuration.pop('table_stats', false)
        self.do_index_stats = configuration.pop('index_stats', false)
        self.databases_to_poll = configuration.pop('database_poll', none)
        self.configuration = configuration
        self.conn = none
        self.conn_params = dict()
        self.server_version = none
        self.is_superuser = false
        self.alive = false
        self.databases = list()
        self.secondaries = list()
        self.replication_slots = list()
        self.queries = dict()
        self.data = dict()

    def reconnect(self):
        return self.connect()

    def build_conn_params(self):
        conf = self.configuration

        # connection uris: https://www.postgresql.org/docs/current/libpq-connect.html#libpq-connstring
        if conf.get(conn_param_dsn):
            return {'dsn': conf[conn_param_dsn]}

        params = {
            conn_param_host: conf.get(conn_param_host),
            conn_param_port: conf.get(conn_param_port, default_port),
            conn_param_database: conf.get(conn_param_database),
            conn_param_user: conf.get(conn_param_user, default_user),
            conn_param_password: conf.get(conn_param_password),
            conn_param_conn_timeout: conf.get(conn_param_conn_timeout, default_connect_timeout),
            'options': '-c statement_timeout={0}'.format(
                conf.get(conn_param_statement_timeout, default_statement_timeout)),
        }

        # https://www.postgresql.org/docs/current/libpq-ssl.html
        ssl_params = dict(
            (k, v) for k, v in {
                conn_param_ssl_mode: conf.get(conn_param_ssl_mode),
                conn_param_ssl_root_cert: conf.get(conn_param_ssl_root_cert),
                conn_param_ssl_crl: conf.get(conn_param_ssl_crl),
                conn_param_ssl_cert: conf.get(conn_param_ssl_cert),
                conn_param_ssl_key: conf.get(conn_param_ssl_key),
            }.items() if v)

        if conn_param_ssl_mode not in ssl_params and len(ssl_params) > 0:
            raise valueerror("mandatory 'sslmode' param is missing, please set")

        params.update(ssl_params)

        return params

    def connect(self):
        if self.conn:
            self.conn.close()
            self.conn = none

        try:
            self.conn = psycopg2.connect(**self.conn_params)
            self.conn.set_isolation_level(extensions.isolation_level_autocommit)
            self.conn.set_session(readonly=true)
        except operationalerror as error:
            self.error(error)
            self.alive = false
        else:
            self.alive = true

        return self.alive

    def check(self):
        if not psycopg2:
            self.error("'python-psycopg2' package is needed to use postgres module")
            return false

        try:
            self.conn_params = self.build_conn_params()
        except valueerror as error:
            self.error('error on creating connection params : {0}', error)
            return false

        if not self.connect():
            self.error('failed to connect to {0}'.format(hide_password(self.conn_params)))
            return false

        try:
            self.check_queries()
        except exception as error:
            self.error(error)
            return false

        self.populate_queries()
        self.create_dynamic_charts()

        return true

    def get_data(self):
        if not self.alive and not self.reconnect():
            return none

        self.data = dict()
        try:
            cursor = self.conn.cursor(cursor_factory=dictcursor)

            self.data.update(zero_lock_types(self.databases))
            for query, metrics in self.queries.items():
                self.query_stats(cursor, query, metrics)

        except operationalerror:
            self.alive = false
            return none

        cursor.close()

        return self.data

    def query_stats(self, cursor, query, metrics):
        cursor.execute(query, dict(databases=tuple(self.databases)))

        for row in cursor:
            for metric in metrics:
                #  databases
                if 'database_name' in row:
                    dimension_id = '_'.join([row['database_name'], metric])
                #  secondaries
                elif 'application_name' in row:
                    dimension_id = '_'.join([row['application_name'], metric])
                # replication slots
                elif 'slot_name' in row:
                    dimension_id = '_'.join([row['slot_name'], metric])
                #  other
                else:
                    dimension_id = metric

                if metric in row:
                    if row[metric] is not none:
                        self.data[dimension_id] = int(row[metric])
                elif 'locks_count' in row:
                    if metric == row['mode']:
                        self.data[dimension_id] = row['locks_count']

    def check_queries(self):
        cursor = self.conn.cursor()

        self.server_version = detect_server_version(cursor, query_factory(query_name_server_version))
        self.debug('server version: {0}'.format(self.server_version))

        self.is_superuser = check_if_superuser(cursor, query_factory(query_name_if_superuser))
        self.debug('superuser: {0}'.format(self.is_superuser))

        self.databases = discover(cursor, query_factory(query_name_databases))
        self.debug('discovered databases {0}'.format(self.databases))
        if self.databases_to_poll:
            to_poll = self.databases_to_poll.split()
            self.databases = [db for db in self.databases if db in to_poll] or self.databases

        self.secondaries = discover(cursor, query_factory(query_name_standby))
        self.debug('discovered secondaries: {0}'.format(self.secondaries))

        if self.server_version >= 94000:
            self.replication_slots = discover(cursor, query_factory(query_name_replication_slot))
            self.debug('discovered replication slots: {0}'.format(self.replication_slots))

        cursor.close()

    def populate_queries(self):
        self.queries[query_factory(query_name_database)] = metrics[query_name_database]
        self.queries[query_factory(query_name_backends)] = metrics[query_name_backends]
        self.queries[query_factory(query_name_backend_usage, self.server_version)] = metrics[query_name_backend_usage]
        self.queries[query_factory(query_name_locks)] = metrics[query_name_locks]
        self.queries[query_factory(query_name_bgwriter)] = metrics[query_name_bgwriter]
        self.queries[query_factory(query_name_diff_lsn, self.server_version)] = metrics[query_name_wal_writes]
        self.queries[query_factory(query_name_standby_delta, self.server_version)] = metrics[query_name_standby_delta]
        self.queries[query_factory(query_name_blockers, self.server_version)] = metrics[query_name_blockers]

        if self.do_index_stats:
            self.queries[query_factory(query_name_index_stats)] = metrics[query_name_index_stats]
        if self.do_table_stats:
            self.queries[query_factory(query_name_table_stats)] = metrics[query_name_table_stats]

        if self.is_superuser:
            self.queries[query_factory(query_name_archive, self.server_version)] = metrics[query_name_archive]

            if self.server_version >= 90400:
                self.queries[query_factory(query_name_wal, self.server_version)] = metrics[query_name_wal]

            if self.server_version >= 100000:
                v = metrics[query_name_repslot_files]
                self.queries[query_factory(query_name_repslot_files, self.server_version)] = v

        if self.server_version >= 90400:
            self.queries[query_factory(query_name_autovacuum)] = metrics[query_name_autovacuum]

        self.queries[query_factory(query_name_emergency_autovacuum)] = metrics[query_name_emergency_autovacuum]
        self.queries[query_factory(query_name_tx_wraparound)] = metrics[query_name_tx_wraparound]

        if self.server_version >= 100000:
            self.queries[query_factory(query_name_standby_lag)] = metrics[query_name_standby_lag]

    def create_dynamic_charts(self):
        for database_name in self.databases[::-1]:
            dim = [
                database_name + '_size',
                database_name,
                'absolute',
                1,
                1024 * 1024,
            ]
            self.definitions['database_size']['lines'].append(dim)
            for chart_name in [name for name in self.order if name.startswith('db_stat')]:
                add_database_stat_chart(
                    order=self.order,
                    definitions=self.definitions,
                    name=chart_name,
                    database_name=database_name,
                )
            add_database_lock_chart(
                order=self.order,
                definitions=self.definitions,
                database_name=database_name,
            )

        for application_name in self.secondaries[::-1]:
            add_replication_standby_chart(
                order=self.order,
                definitions=self.definitions,
                name='standby_delta',
                application_name=application_name,
                chart_family='replication delta',
            )
            add_replication_standby_chart(
                order=self.order,
                definitions=self.definitions,
                name='standby_lag',
                application_name=application_name,
                chart_family='replication lag',
            )

        for slot_name in self.replication_slots[::-1]:
            add_replication_slot_chart(
                order=self.order,
                definitions=self.definitions,
                name='replication_slot',
                slot_name=slot_name,
            )


def discover(cursor, query):
    cursor.execute(query)
    result = list()
    for v in [value[0] for value in cursor]:
        if v not in result:
            result.append(v)
    return result


def check_if_superuser(cursor, query):
    cursor.execute(query)
    return cursor.fetchone()[0]


def detect_server_version(cursor, query):
    cursor.execute(query)
    return int(cursor.fetchone()[0])


def zero_lock_types(databases):
    result = dict()
    for database in databases:
        for lock_type in metrics['locks']:
            key = '_'.join([database, lock_type])
            result[key] = 0

    return result


def hide_password(config):
    return dict((k, v if k != 'password' or not v else '*****') for k, v in config.items())


def add_database_lock_chart(order, definitions, database_name):
    def create_lines(database):
        result = list()
        for lock_type in metrics['locks']:
            dimension_id = '_'.join([database, lock_type])
            result.append([dimension_id, lock_type, 'absolute'])
        return result

    chart_name = database_name + '_locks'
    order.insert(-1, chart_name)
    definitions[chart_name] = {
        'options':
            [none, 'locks on db: ' + database_name, 'locks', 'db ' + database_name, 'postgres.db_locks', 'line'],
        'lines': create_lines(database_name)
    }


def add_database_stat_chart(order, definitions, name, database_name):
    def create_lines(database, lines):
        result = list()
        for line in lines:
            new_line = ['_'.join([database, line[0]])] + line[1:]
            result.append(new_line)
        return result

    chart_template = charts[name]
    chart_name = '_'.join([database_name, name])
    order.insert(0, chart_name)
    name, title, units, _, context, chart_type = chart_template['options']
    definitions[chart_name] = {
        'options': [name, title + ': ' + database_name, units, 'db ' + database_name, context, chart_type],
        'lines': create_lines(database_name, chart_template['lines'])}


def add_replication_standby_chart(order, definitions, name, application_name, chart_family):
    def create_lines(standby, lines):
        result = list()
        for line in lines:
            new_line = ['_'.join([standby, line[0]])] + line[1:]
            result.append(new_line)
        return result

    chart_template = charts[name]
    chart_name = '_'.join([application_name, name])
    position = order.index('database_size')
    order.insert(position, chart_name)
    name, title, units, _, context, chart_type = chart_template['options']
    definitions[chart_name] = {
        'options': [name, title + ': ' + application_name, units, chart_family, context, chart_type],
        'lines': create_lines(application_name, chart_template['lines'])}


def add_replication_slot_chart(order, definitions, name, slot_name):
    def create_lines(slot, lines):
        result = list()
        for line in lines:
            new_line = ['_'.join([slot, line[0]])] + line[1:]
            result.append(new_line)
        return result

    chart_template = charts[name]
    chart_name = '_'.join([slot_name, name])
    position = order.index('database_size')
    order.insert(position, chart_name)
    name, title, units, _, context, chart_type = chart_template['options']
    definitions[chart_name] = {
        'options': [name, title + ': ' + slot_name, units, 'replication slot files', context, chart_type],
        'lines': create_lines(slot_name, chart_template['lines'])}
