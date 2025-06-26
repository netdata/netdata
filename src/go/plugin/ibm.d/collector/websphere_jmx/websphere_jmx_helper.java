// SPDX-License-Identifier: GPL-3.0-or-later

// WebSphere JMX Helper
// 
// NOTE: This implementation uses real WebSphere MBean patterns, but attribute names
// may vary between WebSphere versions and require PMI (Performance Monitoring Infrastructure)
// to be enabled for full metrics collection.
//
// Common WebSphere MBean patterns:
// - Thread Pools: WebSphere:type=ThreadPool,name=*,process=*,node=*
// - JDBC: WebSphere:type=J2CConnectionFactory,* or WebSphere:type=DataSource,*
// - JMS: WebSphere:type=SIBQueuePoint,* or WebSphere:j2eeType=JMSDestination,*
// - Applications: WebSphere:type=Application,* or WebSphere:j2eeType=J2EEApplication,*
// - PMI Stats: WebSphere:type=Perf,*

import javax.management.*;
import javax.management.openmbean.CompositeData;
import javax.management.remote.JMXConnector;
import javax.management.remote.JMXConnectorFactory;
import javax.management.remote.JMXServiceURL;
import java.io.BufferedReader;
import java.io.InputStreamReader;
import java.io.IOException;
import java.util.*;
import java.net.MalformedURLException;

public class websphere_jmx_helper {
    
    private static MBeanServerConnection mbsc;
    private static JMXConnector jmxc;
    private static String protocolVersion = "1.0";
    
    public static void main(String[] args) {
        BufferedReader reader = new BufferedReader(new InputStreamReader(System.in));
        
        try {
            while (true) {
                String line = reader.readLine();
                if (line == null) {
                    break; // End of stream
                }
                
                Map<String, Object> command = parseCommand(line);
                String cmd = (String) command.get("command");
                
                if (cmd == null) {
                    sendError("Missing command field", "", true);
                    continue;
                }
                
                switch (cmd) {
                    case "INIT":
                        handleInit(command);
                        break;
                    case "SCRAPE":
                        handleScrape(command);
                        break;
                    case "PING":
                        handlePing();
                        break;
                    case "SHUTDOWN":
                        handleShutdown();
                        return; // Exit
                    default:
                        sendError("Unknown command: " + cmd, "", true);
                }
            }
        } catch (IOException e) {
            sendError("I/O error in main loop", e.toString(), false);
        } catch (RuntimeException e) {
            sendError("Unhandled runtime exception in main loop", e.toString(), false);
        } finally {
            cleanup();
        }
    }
    
    private static Map<String, Object> parseCommand(String json) {
        Map<String, Object> result = new HashMap<>();
        
        try {
            // Simple JSON parser - handles nested objects and arrays
            json = json.trim();
            if (!json.startsWith("{") || !json.endsWith("}")) {
                return result;
            }
            
            // Remove outer braces
            json = json.substring(1, json.length() - 1);
            
            // Split by commas not inside quotes or nested structures
            List<String> pairs = new ArrayList<>();
            StringBuilder current = new StringBuilder();
            int depth = 0;
            boolean inQuotes = false;
            char prevChar = '\0';
            
            for (char c : json.toCharArray()) {
                if (c == '"' && prevChar != '\\') {
                    inQuotes = !inQuotes;
                } else if (!inQuotes) {
                    if (c == '{' || c == '[') depth++;
                    else if (c == '}' || c == ']') depth--;
                    else if (c == ',' && depth == 0) {
                        pairs.add(current.toString());
                        current = new StringBuilder();
                        prevChar = c;
                        continue;
                    }
                }
                current.append(c);
                prevChar = c;
            }
            if (current.length() > 0) {
                pairs.add(current.toString());
            }
            
            // Parse each key-value pair
            for (String pair : pairs) {
                int colonIndex = pair.indexOf(':');
                if (colonIndex == -1) continue;
                
                String key = pair.substring(0, colonIndex).trim();
                String value = pair.substring(colonIndex + 1).trim();
                
                // Remove quotes from key
                if (key.startsWith("\"") && key.endsWith("\"")) {
                    key = key.substring(1, key.length() - 1);
                }
                
                // Parse value
                if (value.startsWith("\"") && value.endsWith("\"")) {
                    // String value
                    result.put(key, value.substring(1, value.length() - 1));
                } else if (value.equals("true") || value.equals("false")) {
                    // Boolean value
                    result.put(key, Boolean.parseBoolean(value));
                } else if (value.matches("-?\\d+")) {
                    // Integer value
                    result.put(key, Integer.parseInt(value));
                } else if (value.matches("-?\\d+\\.\\d+")) {
                    // Double value
                    result.put(key, Double.parseDouble(value));
                } else if (value.startsWith("{")) {
                    // Nested object - store as string for now
                    result.put(key, value);
                } else {
                    // Default to string
                    result.put(key, value);
                }
            }
        } catch (Exception e) {
            sendError("Failed to parse command JSON", e.toString(), true);
        }
        
        return result;
    }
    
    private static void handleInit(Map<String, Object> command) {
        String jmxUrl = null;
        String username = null;
        try {
            jmxUrl = (String) command.get("jmx_url");
            username = (String) command.get("jmx_username");
            String password = (String) command.get("jmx_password");
            String classpath = (String) command.get("jmx_classpath");
            String version = (String) command.get("protocol_version");
            
            if (version != null) {
                protocolVersion = version;
            }
            
            if (jmxUrl == null || jmxUrl.isEmpty()) {
                sendError("JMX URL is required", "Missing required parameter: jmx_url", false);
                return;
            }
            
            // Create JMX connection
            JMXServiceURL url = new JMXServiceURL(jmxUrl);
            Map<String, Object> env = new HashMap<>();
            
            if (username != null && !username.isEmpty()) {
                String[] credentials = {username, password};
                env.put(JMXConnector.CREDENTIALS, credentials);
            }
            
            // Handle RMI/IIOP specifics
            if (jmxUrl.contains("iiop")) {
                env.put("java.naming.factory.initial", "com.ibm.websphere.naming.WsnInitialContextFactory");
            }
            
            jmxc = JMXConnectorFactory.connect(url, env);
            mbsc = jmxc.getMBeanServerConnection();
            
            sendSuccess("JMX connection established", null);
        } catch (MalformedURLException e) {
            sendError("Invalid JMX URL format", "URL: " + jmxUrl + ", Error: " + e.getMessage(), false);
        } catch (SecurityException e) {
            sendError("Authentication failed", "Username: " + username + ", Error: " + e.getMessage(), false);
        } catch (IOException e) {
            sendError("Connection failed", "URL: " + jmxUrl + ", Error: " + e.getMessage() + 
                     ". Check if the server is running and accessible.", false);
        } catch (Exception e) {
            sendError("Initialization failed", e.getClass().getName() + ": " + e.getMessage(), false);
        }
    }
    
    private static void handleScrape(Map<String, Object> command) {
        if (mbsc == null) {
            sendError("Not connected to JMX server", "", true);
            return;
        }
        
        String target = (String) command.get("target");
        if (target == null) {
            sendError("Target is required for SCRAPE command", "", true);
            return;
        }
        
        try {
            Map<String, Object> data = new HashMap<>();
            
            switch (target) {
                case "JVM":
                    data = collectJVMMetrics();
                    break;
                case "THREADPOOLS":
                    data.put("threadPools", collectThreadPools(command));
                    break;
                case "JDBC":
                    data.put("jdbcPools", collectJDBCPools(command));
                    break;
                case "JMS":
                    data.put("jmsDestinations", collectJMSDestinations(command));
                    break;
                case "APPLICATIONS":
                    data.put("applications", collectApplications(command));
                    break;
                default:
                    sendError("Unknown scrape target: " + target, "", true);
                    return;
            }
            
            // Add cluster-aware properties to the data map
            String clusterName = (String) command.get("cluster_name");
            String cellName = (String) command.get("cell_name");
            String nodeName = (String) command.get("node_name");
            String serverType = (String) command.get("server_type");

            if (clusterName != null && !clusterName.isEmpty()) {
                data.put("cluster_name", clusterName);
            }
            if (cellName != null && !cellName.isEmpty()) {
                data.put("cell_name", cellName);
            }
            if (nodeName != null && !nodeName.isEmpty()) {
                data.put("node_name", nodeName);
            }
            if (serverType != null && !serverType.isEmpty()) {
                data.put("server_type", serverType);
            }

            sendSuccess("Metrics collected", data);
        } catch (JMException e) {
            sendError("JMX error during metrics collection", e.toString(), true);
        } catch (IOException e) {
            sendError("Connection error during metrics collection", e.toString(), true);
        }
    }
    
    private static Map<String, Object> collectJVMMetrics() throws Exception {
        Map<String, Object> jvm = new HashMap<>();
        
        try {
            // Memory metrics
            ObjectName memoryMBean = new ObjectName("java.lang:type=Memory");
            CompositeData heapUsage = (CompositeData) mbsc.getAttribute(memoryMBean, "HeapMemoryUsage");
            CompositeData nonHeapUsage = (CompositeData) mbsc.getAttribute(memoryMBean, "NonHeapMemoryUsage");
            
            Map<String, Object> heap = new HashMap<>();
            heap.put("used", heapUsage.get("used"));
            heap.put("committed", heapUsage.get("committed"));
            heap.put("max", heapUsage.get("max"));
            jvm.put("heap", heap);
            
            Map<String, Object> nonheap = new HashMap<>();
            nonheap.put("used", nonHeapUsage.get("used"));
            nonheap.put("committed", nonHeapUsage.get("committed"));
            jvm.put("nonheap", nonheap);
            
            // GC metrics
            ObjectName gcMBean = new ObjectName("java.lang:type=GarbageCollector,*");
            Set<ObjectName> gcBeans = mbsc.queryNames(gcMBean, null);
            
            long totalGcCount = 0;
            long totalGcTime = 0;
            for (ObjectName gc : gcBeans) {
                totalGcCount += (Long) mbsc.getAttribute(gc, "CollectionCount");
                totalGcTime += (Long) mbsc.getAttribute(gc, "CollectionTime");
            }
            
            Map<String, Object> gc = new HashMap<>();
            gc.put("count", totalGcCount);
            gc.put("time", totalGcTime);
            jvm.put("gc", gc);
            
            // Thread metrics
            ObjectName threadMBean = new ObjectName("java.lang:type=Threading");
            Map<String, Object> threads = new HashMap<>();
            threads.put("count", mbsc.getAttribute(threadMBean, "ThreadCount"));
            threads.put("daemon", mbsc.getAttribute(threadMBean, "DaemonThreadCount"));
            threads.put("peak", mbsc.getAttribute(threadMBean, "PeakThreadCount"));
            threads.put("totalStarted", mbsc.getAttribute(threadMBean, "TotalStartedThreadCount"));
            
            // Deadlock detection
            long[] deadlockedThreadIds = (long[]) mbsc.invoke(threadMBean, "findDeadlockedThreads", null, null);
            threads.put("deadlocked", deadlockedThreadIds != null ? deadlockedThreadIds.length : 0);
            
            jvm.put("threads", threads);
            
            // Class loading metrics
            ObjectName classMBean = new ObjectName("java.lang:type=ClassLoading");
            Map<String, Object> classes = new HashMap<>();
            classes.put("loaded", mbsc.getAttribute(classMBean, "LoadedClassCount"));
            classes.put("unloaded", mbsc.getAttribute(classMBean, "UnloadedClassCount"));
            jvm.put("classes", classes);
        } catch (MalformedObjectNameException e) {
            sendError("Invalid MBean ObjectName for JVM metrics", e.getMessage(), true);
        } catch (JMException e) {
            sendError("JMX error collecting JVM metrics", e.getMessage(), true);
        } catch (IOException e) {
            sendError("Connection error collecting JVM metrics", e.getMessage(), true);
        }
        
        return jvm;
    }
    
    private static List<Map<String, Object>> collectThreadPools(Map<String, Object> command) throws Exception {
        List<Map<String, Object>> pools = new ArrayList<>();
        int maxItems = getMaxItems(command);
        
        ObjectName pattern;
        try {
            // Real WebSphere thread pool MBeans include node and process attributes
            // Pattern: WebSphere:type=ThreadPool,name=<poolName>,process=<processName>,node=<nodeName>,*
            pattern = new ObjectName("WebSphere:type=ThreadPool,*");
        } catch (MalformedObjectNameException e) {
            sendError("Invalid MBean ObjectName for ThreadPools", e.getMessage(), true);
            return pools;
        }

        try {
            Set<ObjectName> poolBeans = mbsc.queryNames(pattern, null);
            
            int count = 0;
            for (ObjectName poolBean : poolBeans) {
                if (maxItems > 0 && count >= maxItems) break;
                
                Map<String, Object> pool = new HashMap<>();
                pool.put("name", poolBean.getKeyProperty("name"));
                pool.put("node", poolBean.getKeyProperty("node"));
                pool.put("process", poolBean.getKeyProperty("process"));
                
                // WebSphere uses different attribute names than the fictional ones
                try {
                    pool.put("poolSize", mbsc.getAttribute(poolBean, "size"));
                    pool.put("activeCount", mbsc.getAttribute(poolBean, "activeThreads"));
                    pool.put("maximumPoolSize", mbsc.getAttribute(poolBean, "maximumSize"));
                } catch (AttributeNotFoundException e) {
                    // Try alternative attribute names used by WebSphere
                    try {
                        pool.put("poolSize", mbsc.getAttribute(poolBean, "poolSize"));
                        pool.put("activeCount", mbsc.getAttribute(poolBean, "activeCount"));
                        pool.put("maximumPoolSize", mbsc.getAttribute(poolBean, "maximumPoolSize"));
                    } catch (AttributeNotFoundException ex) {
                        pool.put("attributes_unavailable", true);
                        sendError("ThreadPool attributes not found for " + poolBean.getCanonicalName(), ex.getMessage(), true);
                    }
                }
                
                pools.add(pool);
                count++;
            }
        } catch (JMException e) {
            sendError("JMX error collecting ThreadPools", e.getMessage(), true);
        } catch (IOException e) {
            sendError("Connection error collecting ThreadPools", e.getMessage(), true);
        }
        
        return pools;
    }
    
    private static List<Map<String, Object>> collectJDBCPools(Map<String, Object> command) throws Exception {
        List<Map<String, Object>> pools = new ArrayList<>();
        int maxItems = getMaxItems(command);
        
        ObjectName patternJ2C;
        ObjectName patternDataSource;
        try {
            // Real WebSphere JDBC connection pool MBeans use J2CConnectionFactory
            // Pattern: WebSphere:type=J2CConnectionFactory,*
            patternJ2C = new ObjectName("WebSphere:type=J2CConnectionFactory,*");
            patternDataSource = new ObjectName("WebSphere:type=DataSource,*");
        } catch (MalformedObjectNameException e) {
            sendError("Invalid MBean ObjectName for JDBC Pools", e.getMessage(), true);
            return pools;
        }

        try {
            Set<ObjectName> poolBeans = mbsc.queryNames(patternJ2C, null);
            
            // Also try DataSource pattern as alternative
            if (poolBeans.isEmpty()) {
                poolBeans = mbsc.queryNames(patternDataSource, null);
            }
            
            int count = 0;
            for (ObjectName poolBean : poolBeans) {
                if (maxItems > 0 && count >= maxItems) break;
                
                Map<String, Object> pool = new HashMap<>();
                pool.put("name", poolBean.getKeyProperty("name"));
                pool.put("jndiName", poolBean.getKeyProperty("jndiName"));
                
                // Connection pool statistics - WebSphere may require PMI to be enabled
                try {
                    pool.put("poolSize", mbsc.getAttribute(poolBean, "poolSize"));
                    pool.put("freePoolSize", mbsc.getAttribute(poolBean, "freePoolSize"));
                    pool.put("waitingThreadCount", mbsc.getAttribute(poolBean, "waitingThreadCount"));
                    pool.put("connectionTimeout", mbsc.getAttribute(poolBean, "connectionTimeout"));
                } catch (AttributeNotFoundException e) {
                    // These metrics might require PMI (Performance Monitoring Infrastructure)
                    pool.put("pmi_required", true);
                }
                
                pools.add(pool);
                count++;
            }
        } catch (JMException e) {
            sendError("JMX error collecting JDBC Pools", e.getMessage(), true);
        } catch (IOException e) {
            sendError("Connection error collecting JDBC Pools", e.getMessage(), true);
        }
        
        return pools;
    }
    
    private static List<Map<String, Object>> collectJMSDestinations(Map<String, Object> command) throws Exception {
        List<Map<String, Object>> destinations = new ArrayList<>();
        int maxItems = getMaxItems(command);
        
        ObjectName patternSIB;
        ObjectName patternJ2EE;
        try {
            // Real WebSphere JMS MBeans use different patterns
            // Try SIB (Service Integration Bus) queue points first
            patternSIB = new ObjectName("WebSphere:type=SIBQueuePoint,*");
            patternJ2EE = new ObjectName("WebSphere:j2eeType=JMSDestination,*");
        } catch (MalformedObjectNameException e) {
            sendError("Invalid MBean ObjectName for JMS Destinations", e.getMessage(), true);
            return destinations;
        }

        try {
            Set<ObjectName> destBeans = mbsc.queryNames(patternSIB, null);
            
            // Also try J2EE standard JMS destination pattern
            if (destBeans.isEmpty()) {
                destBeans = mbsc.queryNames(patternJ2EE, null);
            }
            
            int count = 0;
            for (ObjectName destBean : destBeans) {
                if (maxItems > 0 && count >= maxItems) break;
                
                Map<String, Object> dest = new HashMap<>();
                dest.put("name", destBean.getKeyProperty("name"));
                dest.put("identifier", destBean.getKeyProperty("identifier"));
                
                // SIB Queue Point attributes
                try {
                    dest.put("depth", mbsc.getAttribute(destBean, "Depth"));
                    dest.put("maxDepth", mbsc.getAttribute(destBean, "MaxDepth"));
                    dest.put("state", mbsc.getAttribute(destBean, "State"));
                } catch (AttributeNotFoundException e) {
                    // Try generic JMS attributes
                    try {
                        dest.put("messagesCurrentCount", mbsc.getAttribute(destBean, "messagesCurrentCount"));
                        dest.put("consumerCount", mbsc.getAttribute(destBean, "consumerCount"));
                    } catch (AttributeNotFoundException ex) {
                        dest.put("attributes_unavailable", true);
                    }
                }
                
                destinations.add(dest);
                count++;
            }
        } catch (JMException e) {
            sendError("JMX error collecting JMS Destinations", e.getMessage(), true);
        } catch (IOException e) {
            sendError("Connection error collecting JMS Destinations", e.getMessage(), true);
        }
        
        return destinations;
    }
    
    private static List<Map<String, Object>> collectApplications(Map<String, Object> command) throws Exception {
        List<Map<String, Object>> applications = new ArrayList<>();
        int maxItems = getMaxItems(command);
        
        @SuppressWarnings("unchecked")
        Map<String, Boolean> options = (Map<String, Boolean>) command.get("collect_options");
        boolean collectSessions = options != null && Boolean.TRUE.equals(options.get("sessions"));
        boolean collectTransactions = options != null && Boolean.TRUE.equals(options.get("transactions"));
        
        ObjectName patternApp;
        ObjectName patternJ2EEApp;
        try {
            // Real WebSphere application MBeans
            patternApp = new ObjectName("WebSphere:type=Application,*");
            patternJ2EEApp = new ObjectName("WebSphere:j2eeType=J2EEApplication,*");
        } catch (MalformedObjectNameException e) {
            sendError("Invalid MBean ObjectName for Applications", e.getMessage(), true);
            return applications;
        }

        try {
            Set<ObjectName> appBeans = mbsc.queryNames(patternApp, null);
            
            // Also try J2EE standard pattern
            if (appBeans.isEmpty()) {
                appBeans = mbsc.queryNames(patternJ2EEApp, null);
            }
            
            int count = 0;
            for (ObjectName appBean : appBeans) {
                if (maxItems > 0 && count >= maxItems) break;
                
                Map<String, Object> app = new HashMap<>();
                app.put("name", appBean.getKeyProperty("name"));
                
                try {
                    app.put("requestCount", mbsc.getAttribute(appBean, "requestCount"));
                    app.put("errorCount", mbsc.getAttribute(appBean, "errorCount"));
                    app.put("avgResponseTime", mbsc.getAttribute(appBean, "avgResponseTime"));
                    app.put("maxResponseTime", mbsc.getAttribute(appBean, "maxResponseTime"));
                } catch (AttributeNotFoundException e) {
                    app.put("metrics_unavailable", true);
                }
                
                if (collectSessions) {
                    try {
                        app.put("activeSessions", mbsc.getAttribute(appBean, "activeSessions"));
                        app.put("liveSessions", mbsc.getAttribute(appBean, "liveSessions"));
                        app.put("invalidatedSessions", mbsc.getAttribute(appBean, "invalidatedSessions"));
                    } catch (AttributeNotFoundException e) {
                        app.put("sessions_metrics_unavailable", true);
                    }
                }
                
                if (collectTransactions) {
                    try {
                        app.put("activeTransactions", mbsc.getAttribute(appBean, "activeTransactions"));
                        app.put("committedTransactions", mbsc.getAttribute(appBean, "committedTransactions"));
                        app.put("rolledbackTransactions", mbsc.getAttribute(appBean, "rolledbackTransactions"));
                        app.put("timedoutTransactions", mbsc.getAttribute(appBean, "timedoutTransactions"));
                    } catch (AttributeNotFoundException e) {
                        app.put("transactions_metrics_unavailable", true);
                    }
                }
                
                applications.add(app);
                count++;
            }
        } catch (JMException e) {
            sendError("JMX error collecting Applications", e.getMessage(), true);
        } catch (IOException e) {
            sendError("Connection error collecting Applications", e.getMessage(), true);
        }
        
        return applications;
    }
    
    private static void handlePing() {
        if (mbsc == null) {
            sendError("Not connected to JMX server", "", true);
            return;
        }
        
        try {
            // Test connection by getting the default domain
            String defaultDomain = mbsc.getDefaultDomain();
            sendSuccess("Connection is healthy", null);
        } catch (JMException e) {
            sendError("JMX error during connection test", e.toString(), true);
        } catch (IOException e) {
            sendError("Connection error during connection test", e.toString(), true);
        }
    }
    
    private static void handleShutdown() {
        cleanup();
        sendSuccess("Shutting down", null);
    }
    
    private static void cleanup() {
        if (jmxc != null) {
            try {
                jmxc.close();
            } catch (IOException e) {
                // Ignore
            }
        }
        mbsc = null;
        jmxc = null;
    }
    
    private static int getMaxItems(Map<String, Object> command) {
        Object maxItems = command.get("max_items");
        if (maxItems instanceof Integer) {
            return (Integer) maxItems;
        }
        return 0;
    }
    
    private static void sendSuccess(String message, Map<String, Object> data) {
        Map<String, Object> response = new HashMap<>();
        response.put("status", "OK");
        response.put("message", message);
        if (data != null) {
            response.put("data", data);
        }
        sendResponse(response);
    }
    
    private static void sendError(String message, String details, boolean recoverable) {
        Map<String, Object> response = new HashMap<>();
        response.put("status", "ERROR");
        response.put("message", message);
        if (details != null && !details.isEmpty()) {
            response.put("details", details);
        }
        response.put("recoverable", recoverable);
        sendResponse(response);
    }
    
    private static void sendResponse(Map<String, Object> response) {
        try {
            // Simple JSON serializer
            System.out.println(toJson(response));
            System.out.flush();
        } catch (Exception e) {
            System.err.println("Failed to send response: " + e.toString());
        }
    }
    
    private static String toJson(Object obj) {
        if (obj == null) {
            return "null";
        } else if (obj instanceof String) {
            return "\"" + escapeJson((String) obj) + "\"";
        } else if (obj instanceof Number || obj instanceof Boolean) {
            return obj.toString();
        } else if (obj instanceof Map) {
            @SuppressWarnings("unchecked")
            Map<String, Object> map = (Map<String, Object>) obj;
            StringBuilder sb = new StringBuilder("{");
            boolean first = true;
            for (Map.Entry<String, Object> entry : map.entrySet()) {
                if (!first) sb.append(",");
                sb.append("\"").append(escapeJson(entry.getKey())).append("\":");
                sb.append(toJson(entry.getValue()));
                first = false;
            }
            sb.append("}");
            return sb.toString();
        } else if (obj instanceof List) {
            @SuppressWarnings("unchecked")
            List<Object> list = (List<Object>) obj;
            StringBuilder sb = new StringBuilder("[");
            boolean first = true;
            for (Object item : list) {
                if (!first) sb.append(",");
                sb.append(toJson(item));
                first = false;
            }
            sb.append("]");
            return sb.toString();
        } else {
            return "\"" + escapeJson(obj.toString()) + "\"";
        }
    }
    
    private static String escapeJson(String str) {
        return str.replace("\\", "\\\\")
                  .replace("\"", "\\\"")
                  .replace("\b", "\\b")
                  .replace("\f", "\\f")
                  .replace("\n", "\\n")
                  .replace("\r", "\\r")
                  .replace("\t", "\\t");
    }
}