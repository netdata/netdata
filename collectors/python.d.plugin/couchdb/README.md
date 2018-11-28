# couchdb

This module monitors vital statistics of a local Apache CouchDB 2.x server, including:

* Overall server reads/writes
* HTTP traffic breakdown
  * Request methods (`GET`, `PUT`, `POST`, etc.)
  * Response status codes (`200`, `201`, `4xx`, etc.)
* Active server tasks
* Replication status (CouchDB 2.1 and up only)
* Erlang VM stats
* Optional per-database statistics: sizes, # of docs, # of deleted docs

### Configuration

Sample for a local server running on port 5984:
```yaml
local:
  user: 'admin'
  pass: 'password'
  node: 'couchdb@127.0.0.1'
```

Be sure to specify a correct admin-level username and password.

You may also need to change the `node` name; this should match the value of `-name NODENAME` in your CouchDB's `etc/vm.args` file. Typically this is of the form `couchdb@fully.qualified.domain.name` in a cluster, or `couchdb@127.0.0.1` / `couchdb@localhost` for a single-node server.

If you want per-database statistics, these need to be added to the configuration, separated by spaces:
```yaml
local:
  ...
  databases: 'db1 db2 db3 ...'
```

---
