# WebSphere PMI Collector

This collector monitors IBM WebSphere Application Server using the Performance Monitoring Infrastructure (PMI) via the PerfServlet HTTP interface.

## Overview

The WebSphere PMI collector connects to WebSphere's PerfServlet endpoint to gather performance metrics. Unlike the JMX collector, this uses HTTP/XML for communication, making it easier to configure in some environments.

## Features

- **JVM Metrics**: Heap memory, garbage collection, threads, CPU usage
- **Thread Pools**: Size, active threads, hung threads
- **JDBC Pools**: Connection usage, wait times, pool statistics
- **JCA Pools**: J2C connection factory statistics
- **Web Applications**: Request rates, response times, sessions
- **Servlets**: Request counts, response times (APM)
- **EJBs**: Method invocations, response times, bean pools (APM)
- **Clustering**: Cluster member status and replication metrics

## Requirements

1. **WebSphere Application Server** with PMI enabled
2. **PerfServlet** deployed and accessible (usually at `/wasPerfTool/servlet/perfservlet`)
3. **Network access** to the PerfServlet URL
4. **Authentication credentials** if security is enabled

## Configuration

Edit the `websphere_pmi.conf` configuration file:

```yaml
jobs:
  - name: local_websphere
    url: http://localhost:9080/wasPerfTool/servlet/perfservlet
    username: wasadmin
    password: secret
    pmi_stats_type: extended
```

### Key Configuration Options

- `url`: The PerfServlet URL (required)
- `username`/`password`: Authentication credentials
- `pmi_stats_type`: Statistics level (basic, extended, all, custom)
- `collect_*_metrics`: Enable/disable specific metric categories
- `max_*`: Cardinality limits to prevent memory issues
- `collect_*_matching`: Filter patterns for selective collection

## PMI Setup in WebSphere

1. Open WebSphere Admin Console
2. Navigate to **Monitoring and Tuning > Performance Monitoring Infrastructure (PMI)**
3. Select your server
4. Enable PMI and set the monitoring level
5. Apply changes and restart if necessary

## Metrics

The collector provides metrics in these categories:

- **JVM**: Memory usage, GC activity, thread counts
- **Thread Pools**: WebContainer, Default, ORB pools
- **JDBC**: Connection pool usage and performance
- **Applications**: Request rates and response times
- **Infrastructure**: Session counts, transaction rates

## Performance Considerations

- The `extended` PMI level provides a good balance of metrics vs overhead
- Use `basic` for minimal overhead in production
- Enable APM metrics (servlets, EJBs) selectively due to higher overhead
- Apply cardinality limits in large environments

## Troubleshooting

1. **Cannot connect to PerfServlet**
   - Verify the URL is correct
   - Check if PMI is enabled
   - Ensure PerfServlet is deployed

2. **No metrics collected**
   - Check PMI statistics level
   - Verify XML response contains data
   - Check Netdata logs for errors

3. **Authentication failures**
   - Verify credentials
   - Ensure user has Monitor role
   - Check security configuration

## Differences from JMX Collector

| Feature | PMI Collector | JMX Collector |
|---------|--------------|---------------|
| Protocol | HTTP/XML | RMI/IIOP |
| Setup | Simpler | More complex |
| Firewall | HTTP-friendly | May require multiple ports |
| Performance | Slightly slower | More efficient |
| Features | Core metrics | Full MBean access |