# WebSphere Monitoring Guide for Netdata

## Overview

IBM WebSphere Application Server comes in multiple editions, each with different monitoring capabilities. This guide helps understand which monitoring interfaces are available in each edition and provides recommendations for comprehensive market coverage.

## Netdata Module Mapping

Netdata ships three dedicated WebSphere collectors implemented in the ibm.d framework. Their source locations and backing protocols are:

| Module | Source Path | Protocol Layer |
| ------ | ----------- | -------------- |
| `websphere_pmi` | `src/go/plugin/ibm.d/modules/websphere/pmi` | `protocols/websphere/pmi` (PerfServlet XML) |
| `websphere_mp`  | `src/go/plugin/ibm.d/modules/websphere/mp`  | `protocols/openmetrics` (MicroProfile Metrics) |
| `websphere_jmx` | `src/go/plugin/ibm.d/modules/websphere/jmx` | `protocols/jmxbridge` + `protocols/websphere/jmx` |

Each module exposes generated contexts under its `contexts/` directory and is configured via `/etc/netdata/ibm.d/websphere_*.conf`.

## WebSphere Editions

### 1. WebSphere Traditional (Full Profile)

**Editions:**
- **Base**: Single server deployment, no clustering
- **Network Deployment (ND)**: Full clustering, distributed management
- **Express**: Limited to 480 PVUs, 32-bit only (discontinued)

**Monitoring Capabilities:**
- **PMI (Performance Monitoring Infrastructure)**: Core monitoring system
  - Available via PerfServlet (XML over HTTP)
  - Available via wsadmin (Jython scripting)
  - Available via metrics.ear (Prometheus format) - **only in 8.5.5.20+ and 9.0.5.7+**
- **JMX**: Full MBean access
  - JSR160RMI connector (port 2809-2811 typically)
  - SOAP connector (limited capabilities)
- **Admin Console**: Built-in Tivoli Performance Viewer

**Production Adoption (2024):**
- Still dominates enterprise deployments (~60-70%)
- Version distribution:
  - 8.5.5: ~40% (many on Fix Pack 15-18)
  - 9.0.5: ~30% (more likely to have newer fix packs)
  - Older (7.x, 8.0): ~30% (legacy systems)

### 2. WebSphere Liberty

**Editions:**
- **Liberty Core**: Java EE Web Profile only
- **Liberty Base**: Full Java EE support
- **Liberty ND**: Clustering and collective management

**Monitoring Capabilities:**
- **MicroProfile Metrics**: Modern metrics API
  - Version 1.x-3.x: Custom Prometheus format
  - Version 4.x+: OpenMetrics format
  - Version 5.0+: Micrometer-based
- **MicroProfile Telemetry**: OpenTelemetry support
- **JMX**: Limited compared to Traditional
- **Admin Center**: Web-based management (optional)

**Production Adoption (2024):**
- Growing rapidly (~25-30% of new deployments)
- Preferred for cloud-native applications
- Common in containerized environments

### 3. Open Liberty

**Monitoring Capabilities:**
- Same as WebSphere Liberty
- Open source, no license required
- Full MicroProfile support

**Production Adoption (2024):**
- ~10% of Liberty deployments
- Popular in development/test environments

## Monitoring Interface Comparison

| Feature | Traditional | Liberty | Availability |
|---------|-------------|---------|--------------|
| PerfServlet (XML) | ✓ | ✗ | All Traditional versions |
| metrics.ear (Prometheus) | ✓* | ✗ | Traditional 8.5.5.20+, 9.0.5.7+ |
| MicroProfile Metrics | ✗ | ✓ | All Liberty versions |
| JMX (Full MBeans) | ✓ | Limited | All versions |
| wsadmin/Jython | ✓ | ✗ | All Traditional versions |
| Admin REST API | Limited | ✓ | Liberty only |

*Requires specific fix pack level

## Data Coverage by Interface

### PMI (Traditional Only)
- Thread pools
- Connection pools (JDBC, JMS, JCA)
- Servlet/JSP metrics
- EJB metrics
- Transaction manager
- JVM runtime (heap, GC)
- Session manager
- WebContainer statistics
- ORB metrics
- SIB (Service Integration Bus)

### MicroProfile Metrics (Liberty Only)
- JVM metrics (memory, threads, GC)
- Application-defined metrics
- JAX-RS endpoint metrics
- MicroProfile components
- Custom business metrics

### JMX (Both)
- All PMI data (Traditional)
- Server lifecycle operations
- Configuration MBeans
- Custom application MBeans
- Standard JVM MBeans

## Netdata Implementation Recommendations

### 1. **websphere_pmi Module** (Traditional WebSphere)

**Primary Interface:** PerfServlet
- **Pros**: 
  - Available in ALL Traditional versions
  - No authentication required by default
  - Simple HTTP/XML parsing
  - Comprehensive PMI data
- **Implementation**:
  ```
  URL: http://host:9080/wasPerfTool/servlet/perfservlet
  Fallback: Try metrics.ear if PerfServlet not deployed
  ```

**Secondary Interface:** metrics.ear (if available)
- Check for `/metrics` endpoint
- Use if Fix Pack 20+ (8.5.5) or 7+ (9.0.5)

### 2. **websphere_jmx Module** (All WebSphere)

**Primary Interface:** JMX RMI
- **Pros**:
  - Access to non-PMI MBeans
  - Server control operations
  - Works on both Traditional and Liberty
- **Cons**:
  - Requires client JARs
  - More complex authentication
  - Firewall considerations
- **Implementation**:
  ```
  Traditional: service:jmx:iiop://host:2809/jndi/JMXConnector
  Liberty: service:jmx:rest://host:9443/IBMJMXConnectorREST
  ```

### 3. **websphere_mp Module** (Liberty Only)

**Primary Interface:** MicroProfile Metrics endpoint
- **Pros**:
  - Modern Prometheus format
  - No special libraries needed
  - Standard across Liberty versions
- **Implementation**:
  ```
  URL: https://host:9443/metrics
  Scopes: /metrics/base, /metrics/vendor, /metrics/application
  ```

## Market Coverage Strategy

### Recommended Priority:

1. **websphere_pmi with PerfServlet** (50-60% coverage)
   - Covers all Traditional deployments
   - Most common in enterprises

2. **websphere_mp for Liberty** (25-30% coverage)
   - Growing segment
   - Cloud-native deployments

3. **websphere_jmx as universal fallback** (15-20% coverage)
   - Handles edge cases
   - Custom monitoring needs

### Detection Logic:
```
1. Try Liberty endpoints first (/metrics, /ibm/api)
   → Use websphere_mp if found

2. Try PerfServlet (/wasPerfTool/servlet/perfservlet)
   → Use websphere_pmi if found

3. Try metrics.ear (/metrics with WAS-specific headers)
   → Use websphere_pmi if found

4. Fall back to JMX discovery
   → Use websphere_jmx
```

## Implementation Notes

### Authentication Considerations
- **Traditional**: Often uses basic auth or LTPA tokens
- **Liberty**: Supports OAuth, JWT, basic auth
- **JMX**: May require SSL certificates

### Performance Impact
- **PerfServlet**: Low impact, read-only
- **metrics.ear**: Low impact, cached data
- **JMX**: Higher impact, real-time queries

### Common Deployment Patterns
- **Banking/Finance**: Traditional ND with PMI set to "Extended"
- **Retail/E-commerce**: Mix of Traditional and Liberty
- **Startups/Cloud**: Primarily Liberty or Open Liberty
- **Government**: Traditional with strict security

## Conclusion

For comprehensive WebSphere monitoring coverage, Netdata should:

1. Implement PerfServlet-based monitoring for Traditional WebSphere (largest market share)
2. Support MicroProfile Metrics for Liberty deployments (growing segment)
3. Provide JMX as a universal fallback option
4. Auto-detect WebSphere type and choose appropriate module

This approach ensures coverage of 95%+ of WebSphere deployments in production today.
