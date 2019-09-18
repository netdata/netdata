# dnsdist

This module monitors dnsdist performance and health metrics.

The module draws the following charts:

1.  **Response latency**

    -   latency-slow
    -   latency100-1000
    -   latency50-100
    -   latency10-50
    -   latency1-10
    -   latency0-1

2.  **Cache performance**

    -   cache-hits
    -   cache-misses

3.  **ACL events**

    -   acl-drops
    -   rule-drop
    -   rule-nxdomain
    -   rule-refused

4.  **Noncompliant data**

    -   empty-queries
    -   no-policy
    -   noncompliant-queries
    -   noncompliant-responses

5.  **Queries**

    -   queries
    -   rdqueries
    -   rdqueries

6.  **Health**

    -   downstream-send-errors
    -   downstream-timeouts
    -   servfail-responses
    -   trunc-failures

## Configuration

```yaml
localhost:
  name : 'local'
  url  : 'http://127.0.0.1:5053/jsonstat?command=stats'
  user : 'username'
  pass : 'password'
  header:
    X-API-Key: 'dnsdist-api-key'
```

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fcollectors%2Fpython.d.plugin%2Fdnsdist%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
