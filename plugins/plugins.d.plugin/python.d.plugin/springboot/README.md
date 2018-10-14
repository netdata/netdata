# springboot

This module will monitor one or more Java Spring-boot applications depending on configuration.

It produces following charts:

1. **Response Codes** in requests/s
 * 1xx
 * 2xx
 * 3xx
 * 4xx
 * 5xx
 * others

2. **Threads**
 * daemon
 * total

3. **GC Time** in milliseconds and **GC Operations** in operations/s
 * Copy
 * MarkSweep
 * ...

4. **Heap Mmeory Usage** in KB
 * used
 * committed

### configuration

Please see the [Monitoring Java Spring Boot Applications](https://github.com/netdata/netdata/wiki/Monitoring-Java-Spring-Boot-Applications) page for detailed info about module configuration.

---
