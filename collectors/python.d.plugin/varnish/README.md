# varnish

Module uses the `varnishstat` command to provide varnish cache statistics.

It produces:

1. **Connections Statistics** in connections/s
 * accepted
 * dropped

2. **Client Requests** in requests/s
 * received

3. **All History Hit Rate Ratio** in percent
 * hit
 * miss
 * hitpass

4. **Current Poll Hit Rate Ratio** in percent
 * hit
 * miss
 * hitpass

5. **Expired Objects** in expired/s
 * objects

6. **Least Recently Used Nuked Objects** in nuked/s
 * objects


7. **Number Of Threads In All Pools** in threads
 * threads

8. **Threads Statistics** in threads/s
 * created
 * failed
 * limited

9. **Current Queue Length** in requests
 * in queue

10. **Backend Connections Statistics** in connections/s
 * successful
 * unhealthy
 * reused
 * closed
 * resycled
 * failed

10. **Requests To The Backend** in requests/s
 * received

11. **ESI Statistics** in problems/s
 * errors
 * warnings

12. **Memory Usage** in MB
 * free
 * allocated

13. **Uptime** in seconds
 * uptime


### configuration

No configuration is needed.

---
