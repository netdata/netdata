# WebSphere JMX collector

## Overview

The WebSphere JMX collector monitors IBM WebSphere Application Server instances via JMX (Java Management Extensions). It uses a persistent Java helper process to connect to WebSphere's JMX interface, avoiding JVM startup overhead and providing efficient metric collection.

This collector supports both WebSphere Traditional and WebSphere Liberty profiles through JMX connectivity.

### Key Features

- **Enterprise-Ready**: Built-in resilience with circuit breaker pattern and automatic retries
- **Cluster-Aware**: Monitors cluster state, member health, and replication metrics
- **Self-Healing**: Automatic Java helper process monitoring and restart
- **Production-Safe**: Graceful degradation with cached metrics during connection issues
- **Label-Based Aggregation**: Proper cluster/cell/node labeling for dashboard grouping

## Collected metrics

The collector provides comprehensive monitoring across multiple areas:

### JVM Metrics
- **Heap Memory**: Used, committed, and maximum heap sizes with usage percentage
- **Non-Heap Memory**: Metaspace and other non-heap memory usage
- **Garbage Collection**: Collection count and time spent in GC
- **Threads**: Thread count, daemon threads, peak threads, and total started
- **Class Loading**: Loaded and unloaded class counts

### Thread Pool Metrics
- **Pool Size**: Current and maximum thread pool sizes
- **Active Threads**: Number of threads currently processing requests
- **Pool Utilization**: Percentage of pool capacity being used

### JDBC Connection Pool Metrics
- **Pool Size**: Total connections in the pool
- **Active/Free Connections**: Breakdown of connection usage
- **Wait Time**: Average time waiting for a connection
- **Use Time**: Average time connections are held
- **Connection Lifecycle**: Total connections created and destroyed

### JMS Destination Metrics
- **Message Counts**: Current, pending, and total messages processed
- **Consumer Count**: Number of active message consumers
- **Queue/Topic Support**: Monitoring for both queue and topic destinations

### Web Application Metrics
- **Request Statistics**: Total requests and error counts
- **Response Time**: Average and maximum response times
- **HTTP Sessions**: Active, live, and invalidated session counts
- **Transaction Metrics**: Active, committed, rolled back, and timed out transactions

### Cluster Metrics (Deployment Manager Only)
- **Cluster State**: Running, partial, or stopped status
- **Member Health**: Running vs target member count
- **High Availability**: HAManager coordinator status and core group size
- **Replication**: Traffic rates and sync failures
- **Dynamic Clusters**: Min/max/target instance configuration

### Application Performance Monitoring (APM) Features
- **Servlet Metrics**: Per-servlet response times, request counts, error rates, and concurrent requests
- **EJB Metrics**: Method invocation rates, response times, and bean pool utilization
- **Advanced JDBC Metrics**: Query execution time vs connection hold time breakdown, statement cache performance

### Connection Health Metrics
- **Connection Status**: Real-time JMX connection health
- **Circuit Breaker State**: Closed, half-open, or open status
- **Data Staleness**: Age of cached metrics during failures

## Configuration

### Prerequisites

1. **JMX enabled on WebSphere**: Configure JMX connectivity in the WebSphere admin console
2. **Java Runtime**: Java 8+ must be installed and available in PATH
3. **Network Access**: Connectivity to the WebSphere JMX port (typically 9999 for RMI or 2809 for IIOP)
4. **WebSphere Client Libraries** (Traditional WAS only): Required for IIOP protocol connections
5. **PMI enabled** (for APM features): Performance Monitoring Infrastructure must be enabled for servlet and EJB metrics

### Basic Configuration

```yaml
jobs:
  - name: local_liberty
    jmx_url: service:jmx:rmi:///jndi/rmi://localhost:9999/jmxrmi
    
  - name: remote_traditional
    jmx_url: service:jmx:iiop://washost.example.com:2809/jndi/JMXConnector
    jmx_username: wasadmin
    jmx_password: mypassword
    jmx_classpath: /opt/IBM/WebSphere/AppServer/runtimes/*
```

### Cluster-Aware Configuration

For enterprise deployments, configure cluster labels for proper aggregation:

```yaml
jobs:
  # Application servers
  - name: prod_app_server_1
    jmx_url: service:jmx:iiop://server1:2809/jndi/JMXConnector
    jmx_username: wasadmin
    jmx_password: mypassword
    cluster_name: production_cluster
    cell_name: prod_cell
    node_name: node01
    server_type: app_server
    custom_labels:
      environment: production
      datacenter: us-east-1
      
  # Deployment Manager (for cluster metrics)
  - name: prod_deployment_manager
    jmx_url: service:jmx:iiop://dmgr:9809/jndi/JMXConnector
    jmx_username: wasadmin
    jmx_password: mypassword
    cluster_name: production_cluster
    cell_name: prod_cell
    node_name: dmgr_node
    server_type: dmgr
    collect_cluster_metrics: true
```

### APM Configuration

Enable Application Performance Monitoring features for detailed servlet and EJB visibility:

```yaml
jobs:
  - name: apm_monitoring
    jmx_url: service:jmx:rmi:///jndi/rmi://localhost:9999/jmxrmi
    # Enable APM features (requires PMI to be enabled in WebSphere)
    collect_servlet_metrics: true
    collect_ejb_metrics: true
    collect_jdbc_advanced: true
    # Control APM cardinality
    max_servlets: 100
    max_ejbs: 50
    # Focus on important servlets and EJBs
    collect_servlets_matching: "com.mycompany.api.*"
    collect_ejbs_matching: "OrderProcessing*|PaymentGateway*"
```

### Configuration Options

| Parameter | Description | Default | Required |
|-----------|-------------|---------|----------|
| `jmx_url` | JMX service URL | - | Yes |
| `jmx_username` | Username for authentication | - | No |
| `jmx_password` | Password for authentication | - | No |
| `jmx_classpath` | WebSphere client libraries path | - | No |
| `java_exec_path` | Path to Java executable | `java` | No |
| `jmx_timeout` | JMX operation timeout (seconds) | 5 | No |
| `collect_jvm_metrics` | Enable JVM monitoring | true | No |
| `collect_threadpool_metrics` | Enable thread pool monitoring | true | No |
| `collect_jdbc_metrics` | Enable JDBC pool monitoring | true | No |
| `collect_jms_metrics` | Enable JMS monitoring | true | No |
| `collect_webapp_metrics` | Enable web application monitoring | true | No |
| `collect_session_metrics` | Enable HTTP session monitoring | true | No |
| `collect_transaction_metrics` | Enable JTA transaction monitoring | true | No |
| `collect_cluster_metrics` | Enable cluster metrics (dmgr only) | true | No |
| `collect_servlet_metrics` | Enable per-servlet APM metrics (requires PMI) | false | No |
| `collect_ejb_metrics` | Enable per-EJB APM metrics (requires PMI) | false | No |
| `collect_jdbc_advanced` | Enable advanced JDBC timing breakdown | false | No |

### Cardinality Control

To prevent metric explosion in large environments:

| Parameter | Description | Default |
|-----------|-------------|---------|
| `max_threadpools` | Maximum thread pools to monitor | 50 |
| `max_jdbc_pools` | Maximum JDBC pools to monitor | 50 |
| `max_jms_destinations` | Maximum JMS destinations to monitor | 50 |
| `max_applications` | Maximum applications to monitor | 100 |
| `max_servlets` | Maximum servlets to monitor for APM | 50 |
| `max_ejbs` | Maximum EJBs to monitor for APM | 50 |

### Filtering Options

Use pattern matching to monitor specific resources:

| Parameter | Description | Example |
|-----------|-------------|---------|
| `collect_apps_matching` | Application name pattern | `"prod_*"` |
| `collect_pools_matching` | Pool name pattern | `"*oracle*"` |
| `collect_jms_matching` | JMS destination pattern | `"Queue_*"` |
| `collect_servlets_matching` | Servlet APM pattern | `"com.mycompany.*"` |
| `collect_ejbs_matching` | EJB APM pattern | `"OrderProcessing*"` |

### Cluster Labels

Configure labels for proper cluster aggregation in Netdata dashboards:

| Parameter | Description | Example |
|-----------|-------------|---------|
| `cluster_name` | WebSphere cluster name | `"production_cluster"` |
| `cell_name` | WebSphere cell name | `"prod_cell"` |
| `node_name` | WebSphere node name | `"node01"` |
| `server_type` | Server type | `"app_server"`, `"dmgr"`, `"nodeagent"` |
| `custom_labels` | Additional labels | `{"env": "prod", "region": "us-east"}` |

### Resilience Configuration

Advanced settings for production environments:

| Parameter | Description | Default |
|-----------|-------------|---------|
| `max_retries` | Collection retry attempts | 3 |
| `retry_backoff_multiplier` | Exponential backoff factor | 2.0 |
| `circuit_breaker_threshold` | Failures before circuit opens | 5 |
| `helper_restart_max` | Max helper restarts per hour | 3 |
| `helper_memory_limit_mb` | Java helper memory limit | 512 |

## Requirements

### Software Requirements
- Java 8 or later
- Network connectivity to WebSphere JMX port
- For WebSphere Traditional with IIOP: WebSphere thin client libraries

### WebSphere Configuration

#### Enable JMX (Liberty Profile)
Add to `server.xml`:
```xml
<featureManager>
    <feature>restConnector-2.0</feature>
    <feature>monitor-1.0</feature>
</featureManager>

<remoteFileAccess>
    <writeDir>${server.config.dir}</writeDir>
</remoteFileAccess>
```

#### Enable JMX (Traditional WAS)
1. In the admin console: Servers > Server Types > WebSphere application servers > [server_name]
2. Navigate to: Java and Process Management > Process definition > Java Virtual Machine
3. Add generic JVM arguments:
   ```
   -Dcom.sun.management.jmxremote
   -Dcom.sun.management.jmxremote.port=9999
   -Dcom.sun.management.jmxremote.authenticate=false
   -Dcom.sun.management.jmxremote.ssl=false
   ```
4. For production, enable authentication and SSL

### Security Considerations
- Use authentication in production environments
- Enable SSL for encrypted communication
- Restrict JMX port access with firewalls
- Use dedicated monitoring credentials with minimal privileges

## Troubleshooting

### Common Issues

#### JMX Connection Fails
**Symptoms**: "failed to start JMX helper" or connection timeout errors

**Solutions**:
1. Verify JMX is enabled on WebSphere
2. Check network connectivity: `telnet <host> <port>`
3. Verify firewall allows access to JMX port
4. For Traditional WAS with IIOP, ensure `jmx_classpath` points to WebSphere client libraries
5. Check authentication credentials if security is enabled

#### Missing Metrics for Some Resources
**Symptoms**: Some thread pools, JDBC pools, or applications not appearing

**Possible Causes**:
1. **Cardinality limits reached**: Check logs for "reached max limit" messages
2. **Pattern filtering**: Verify `collect_*_matching` patterns include desired resources
3. **Permissions**: Ensure JMX user has access to all MBeans
4. **Resource state**: Some resources may not be active or deployed

#### Java Helper Process Issues
**Symptoms**: "JAR file not found" or Java process startup failures

**Solutions**:
1. Ensure Java is installed and in PATH
2. For custom Java installations, set `java_exec_path`
3. Run the build script to create the JAR: `./build_java_helper.sh`
4. Check Java version compatibility (requires Java 8+)

#### High Memory Usage
**Symptoms**: Increasing memory consumption over time

**Solutions**:
1. Reduce cardinality limits (`max_*` parameters)
2. Use more restrictive filtering patterns
3. Disable unnecessary metric collection (`collect_*_metrics: false`)
4. Monitor for memory leaks in the Java helper process

#### APM Metrics Not Available
**Symptoms**: Missing servlet or EJB metrics despite enabling APM collection

**Solutions**:
1. **Enable PMI**: Ensure Performance Monitoring Infrastructure is enabled in WebSphere
2. **Set PMI level**: Configure PMI statistical level to "Extended" or "Custom" with servlet/EJB statistics enabled
3. **Check application activity**: APM metrics only appear for actively used servlets and EJBs
4. **Verify permissions**: Ensure JMX user has access to PMI MBeans
5. **Check filtering patterns**: Verify `collect_servlets_matching` and `collect_ejbs_matching` patterns

#### PMI Configuration (Traditional WAS)
In the WebSphere admin console:
1. Navigate to: Monitoring and Tuning > Performance Monitoring Infrastructure (PMI)
2. Select your server and click "Configuration"
3. Set statistical level to "Extended" or "Custom"
4. For Custom level, enable:
   - servletSessionsModule (for servlet metrics)
   - ejbModule (for EJB metrics)
   - connectionPoolModule (for advanced JDBC metrics)

### Performance Tuning

For large WebSphere environments:

1. **Reduce Collection Scope**:
   ```yaml
   collect_jvm_metrics: true
   collect_threadpool_metrics: false
   collect_jdbc_metrics: true
   collect_jms_metrics: false
   collect_webapp_metrics: true
   collect_session_metrics: false
   collect_transaction_metrics: false
   ```

2. **Apply Strict Filtering**:
   ```yaml
   collect_apps_matching: "prod_*"
   max_applications: 20
   max_jdbc_pools: 10
   ```

3. **Increase Collection Interval**:
   ```yaml
   update_every: 10  # Collect every 10 seconds instead of 5
   ```

### Debugging

Enable debug logging in Netdata to see detailed JMX communication:

```bash
# Edit netdata.conf
[global]
    debug flags = 0x0000000000000001

# Or set environment variable
export NETDATA_DEBUG=1
```

Check the Java helper process output in Netdata logs for JMX-specific errors.

### Getting Help

For additional support:
1. Check Netdata community forums
2. Review WebSphere documentation for JMX configuration
3. Verify MBean availability using tools like JConsole or VisualVM
4. Test JMX connectivity independently before configuring Netdata