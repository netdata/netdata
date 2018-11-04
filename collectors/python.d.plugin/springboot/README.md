# springboot

This module will monitor one or more Java Spring-boot applications depending on configuration.
Netdata can be used to monitor running Java [Spring Boot](https://spring.io/) applications that expose their metrics with the use of the **Spring Boot Actuator** included in Spring Boot library.

## Configuration

The Spring Boot Actuator exposes these metrics over HTTP and is very easy to use:
* add `org.springframework.boot:spring-boot-starter-actuator` to your application dependencies
* set `endpoints.metrics.sensitive=false` in your `application.properties`

You can create custom Metrics by add and inject a PublicMetrics in your application.
This is a example to add custom metrics:
```java
package com.example;

import org.springframework.boot.actuate.endpoint.PublicMetrics;
import org.springframework.boot.actuate.metrics.Metric;
import org.springframework.stereotype.Service;

import java.lang.management.ManagementFactory;
import java.lang.management.MemoryPoolMXBean;
import java.util.ArrayList;
import java.util.Collection;

@Service
public class HeapPoolMetrics implements PublicMetrics {

    private static final String PREFIX = "mempool.";
    private static final String KEY_EDEN = PREFIX + "eden";
    private static final String KEY_SURVIVOR = PREFIX + "survivor";
    private static final String KEY_TENURED = PREFIX + "tenured";

    @Override
    public Collection<Metric<?>> metrics() {
        Collection<Metric<?>> result = new ArrayList<>(4);
        for (MemoryPoolMXBean mem : ManagementFactory.getMemoryPoolMXBeans()) {
            String poolName = mem.getName();
            String name = null;
            if (poolName.indexOf("Eden Space") != -1) {
                name = KEY_EDEN;
            } else if (poolName.indexOf("Survivor Space") != -1) {
                name = KEY_SURVIVOR;
            } else if (poolName.indexOf("Tenured Gen") != -1 || poolName.indexOf("Old Gen") != -1) {
                name = KEY_TENURED;
            }

            if (name != null) {
                result.add(newMemoryMetric(name, mem.getUsage().getMax()));
                result.add(newMemoryMetric(name + ".init", mem.getUsage().getInit()));
                result.add(newMemoryMetric(name + ".committed", mem.getUsage().getCommitted()));
                result.add(newMemoryMetric(name + ".used", mem.getUsage().getUsed()));
            }
        }
        return result;
    }

    private Metric<Long> newMemoryMetric(String name, long bytes) {
        return new Metric<>(name, bytes / 1024);
    }
}
```

Please refer [Spring Boot Actuator: Production-ready features](https://docs.spring.io/spring-boot/docs/current/reference/html/production-ready.html) and [81. Actuator - Part IX. ‘How-to’ guides](https://docs.spring.io/spring-boot/docs/current/reference/html/howto-actuator.html) for more information.

## Charts

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

## Usage

The springboot module is enabled by default. It looks up `http://localhost:8080/metrics` and `http://127.0.0.1:8080/metrics` to detect Spring Boot application by default. You can change it by editing `/etc/netdata/python.d/springboot.conf` (to edit it on your system run `/etc/netdata/edit-config python.d/springboot.conf`).

This module defines some common charts, and you can add custom charts by change the configurations.

The configuration format is like:
```yaml
<id>:
  name: '<name>'
  url:  '<metrics endpoint>' # ex. http://localhost:8080/metrics
  user: '<username>'         # optional
  pass: '<password>'         # optional
  defaults:
    [<chart-id>]: true|false
  extras:
  - id: '<chart-id>'
    options:
      title:  '***'
      units:  '***'
      family: '***'
      context: 'springboot.***'
      charttype: 'stacked' | 'area' | 'line'
    lines:
    - { dimension: 'myapp_ok',  name: 'ok',  algorithm: 'absolute', multiplier: 1, divisor: 1} # it shows "myapp.ok" metrics
    - { dimension: 'myapp_ng',  name: 'ng',  algorithm: 'absolute', multiplier: 1, divisor: 1} # it shows "myapp.ng" metrics
```

By default, it creates `response_code`, `threads`, `gc_time`, `gc_ope` abd `heap` charts.
You can disable the default charts by set `defaults.<chart-id>: false`.

The dimension name of extras charts should replace `.` to `_`.

Please check [springboot.conf](https://github.com/netdata/netdata/tree/master/collectors/python.d.plugin/springboot/springboot.conf) for more examples.
