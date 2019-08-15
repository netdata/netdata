# couchdb

This module monitors vital statistics of a local Apache CouchDB 2.x server, including:

-   Overall server reads/writes
-   HTTP traffic breakdown
    -   Request methods (`GET`, `PUT`, `POST`, etc.)
    -   Response status codes (`200`, `201`, `4xx`, etc.)
-   Active server tasks
-   Replication status (CouchDB 2.1 and up only)
-   Erlang VM stats
-   Optional per-database statistics: sizes, # of docs, # of deleted docs

## Configuration

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

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fcollectors%2Fpython.d.plugin%2Fcouchdb%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
