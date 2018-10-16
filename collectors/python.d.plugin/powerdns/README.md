# powerdns

Module monitor powerdns performance and health metrics.

Powerdns charts:

1. **Queries and Answers**
 * udp-queries
 * udp-answers
 * tcp-queries
 * tcp-answers

2. **Cache Usage**
 * query-cache-hit
 * query-cache-miss
 * packetcache-hit
 * packetcache-miss

3. **Cache Size**
 * query-cache-size
 * packetcache-size
 * key-cache-size
 * meta-cache-size

4. **Latency**
 * latency

 Powerdns Recursor charts:

 1. **Questions In**
 * questions
 * ipv6-questions
 * tcp-queries

2. **Questions Out**
 * all-outqueries
 * ipv6-outqueries
 * tcp-outqueries
 * throttled-outqueries

3. **Answer Times**
 * answers-slow
 * answers0-1
 * answers1-10
 * answers10-100
 * answers100-1000

4. **Timeouts**
 * outgoing-timeouts
 * outgoing4-timeouts
 * outgoing6-timeouts

5. **Drops**
 * over-capacity-drops

6. **Cache Usage**
 * cache-hits
 * cache-misses
 * packetcache-hits
 * packetcache-misses

7. **Cache Size**
 * cache-entries
 * packetcache-entries
 * negcache-entries

### configuration

```yaml
local:
  name     : 'local'
  url     : 'http://127.0.0.1:8081/api/v1/servers/localhost/statistics'
  header   :
    X-API-Key: 'change_me'
```

---
