# clickhouse

This module monitors the ClickHouse database server.

It produces metrics based of the following queries:
* "events" category: "SELECT event, value FROM system.events"
* "async" category: "SELECT metric, value FROM system.asynchronous_metrics"
* "metrics" category: "SELECT metric, value FROM system.metrics"

**Requirements:**
ClickHouse allowing access without username and password.

### Configuration

See the comments in the default configuration file.
