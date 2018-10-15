# dnsdist

Module monitor dnsdist performance and health metrics.

Following charts are drawn:

1. **Response latency**
 * latency-slow
 * latency100-1000
 * latency50-100
 * latency10-50
 * latency1-10
 * latency0-1

2. **Cache performance**
 * cache-hits
 * cache-misses

3. **ACL events**
 * acl-drops
 * rule-drop
 * rule-nxdomain
 * rule-refused

4. **Noncompliant data**
 * empty-queries
 * no-policy
 * noncompliant-queries
 * noncompliant-responses

5. **Queries**
 * queries
 * rdqueries
 * rdqueries

6. **Health**
 * downstream-send-errors
 * downstream-timeouts
 * servfail-responses
 * trunc-failures

### configuration

```yaml
localhost:
  name : 'local'
  url  : 'http://127.0.0.1:5053/jsonstat?command=stats'
  user : 'username'
  pass : 'password'
  header:
    X-API-Key: 'dnsdist-api-key'
```

---
