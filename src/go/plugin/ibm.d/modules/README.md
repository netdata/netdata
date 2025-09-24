# IBM.d Modules Directory

Each subdirectory under `modules/` contains a fully self-contained IBM.D collector. A module typically includes:

```
modules/<name>/
├── collector.go          # orchestration logic
├── collect_*.go          # protocol-specific data fetchers
├── config.go             # module configuration
├── contexts/             # declarative metric definitions + generated code
├── module.yaml           # metadata for docgen
├── README.md             # module-specific documentation
├── metadata.yaml         # generated Netdata marketplace entry
├── config_schema.json    # generated JSON schema
└── ...
```

Current modules:

| Module | Monitors | Notes |
|--------|----------|-------|
| `as400` | IBM i/AS/400 systems | Uses the DB2 ODBC bridge; exports system, job, storage metrics. |
| `db2` | IBM DB2 databases (LUW, i, z/OS) | Extensive coverage of buffer pools, locking, tablespaces, memory. |
| `mq` | IBM MQ queue managers | Uses MQ PCF protocol for queues, channels, listeners, stats. |
| `websphere/jmx` | WebSphere via helper JMX bridge | Requires the Java helper shipped with the plugin. |
| `websphere/mp` | WebSphere Liberty / Open Liberty (MicroProfile) | Uses the generic OpenMetrics protocol. |
| `websphere/pmi` | WebSphere Traditional PMI PerfServlet | Parses PMI XML snapshots. |

Common helpers live in [`../framework`](../framework/README.md) and [`../protocols`](../protocols/README.md). When adding a new module, follow the workflow described there and ensure `go generate` keeps metadata in sync.
