// SPDX-License-Identifier: GPL-3.0-or-later

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
        } catch (Exception e) {
            sendError("Unhandled exception in main loop", e.toString(), false);
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
            sendError("Failed to parse command", e.toString(), true);
        }
        
        return result;
    }
    
    private static void handleInit(Map<String, Object> command) {
        try {
            String jmxUrl = (String) command.get("jmx_url");
            String username = (String) command.get("jmx_username");
            String password = (String) command.get("jmx_password");
            String classpath = (String) command.get("jmx_classpath");
            String version = (String) command.get("protocol_version");
            
            if (version != null) {
                protocolVersion = version;
            }
            
            if (jmxUrl == null || jmxUrl.isEmpty()) {
                sendError("JMX URL is required", "", false);
                return;
            }
            
            // Create JMX connection
            JMXServiceURL url = new JMXServiceURL(jmxUrl);
            Map<String, Object> env = new HashMap<>();
            
            if (username != null && !username.isEmpty()) {
                String[] credentials = {username, password};
                env.put(JMXConnector.CREDENTIALS, credentials);
            }
            
            jmxc = JMXConnectorFactory.connect(url, env);
            mbsc = jmxc.getMBeanServerConnection();
            
            sendSuccess("JMX connection established", null);
        } catch (MalformedURLException e) {
            sendError("Invalid JMX URL", e.toString(), false);
        } catch (SecurityException e) {
            sendError("Authentication failed", e.toString(), false);
        } catch (IOException e) {
            sendError("Connection failed", e.toString(), false);
        } catch (Exception e) {
            sendError("Initialization failed", e.toString(), false);
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
            
            sendSuccess("Metrics collected", data);
        } catch (Exception e) {
            sendError("Failed to collect metrics", e.toString(), true);
        }
    }
    
    private static Map<String, Object> collectJVMMetrics() throws Exception {
        Map<String, Object> jvm = new HashMap<>();
        
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
        jvm.put("threads", threads);
        
        // Class loading metrics
        ObjectName classMBean = new ObjectName("java.lang:type=ClassLoading");
        Map<String, Object> classes = new HashMap<>();
        classes.put("loaded", mbsc.getAttribute(classMBean, "LoadedClassCount"));
        classes.put("unloaded", mbsc.getAttribute(classMBean, "UnloadedClassCount"));
        jvm.put("classes", classes);
        
        return jvm;
    }
    
    private static List<Map<String, Object>> collectThreadPools(Map<String, Object> command) throws Exception {
        List<Map<String, Object>> pools = new ArrayList<>();
        int maxItems = getMaxItems(command);
        
        // WebSphere thread pool MBeans
        ObjectName pattern = new ObjectName("WebSphere:type=ThreadPool,*");
        Set<ObjectName> poolBeans = mbsc.queryNames(pattern, null);
        
        int count = 0;
        for (ObjectName poolBean : poolBeans) {
            if (maxItems > 0 && count >= maxItems) break;
            
            Map<String, Object> pool = new HashMap<>();
            pool.put("name", poolBean.getKeyProperty("name"));
            pool.put("poolSize", mbsc.getAttribute(poolBean, "poolSize"));
            pool.put("activeCount", mbsc.getAttribute(poolBean, "activeCount"));
            pool.put("maximumPoolSize", mbsc.getAttribute(poolBean, "maximumPoolSize"));
            
            pools.add(pool);
            count++;
        }
        
        return pools;
    }
    
    private static List<Map<String, Object>> collectJDBCPools(Map<String, Object> command) throws Exception {
        List<Map<String, Object>> pools = new ArrayList<>();
        int maxItems = getMaxItems(command);
        
        // WebSphere JDBC connection pool MBeans
        ObjectName pattern = new ObjectName("WebSphere:type=ConnectionPool,*");
        Set<ObjectName> poolBeans = mbsc.queryNames(pattern, null);
        
        int count = 0;
        for (ObjectName poolBean : poolBeans) {
            if (maxItems > 0 && count >= maxItems) break;
            
            Map<String, Object> pool = new HashMap<>();
            pool.put("name", poolBean.getKeyProperty("name"));
            pool.put("poolSize", mbsc.getAttribute(poolBean, "poolSize"));
            pool.put("numConnectionsUsed", mbsc.getAttribute(poolBean, "numConnectionsUsed"));
            pool.put("numConnectionsFree", mbsc.getAttribute(poolBean, "numConnectionsFree"));
            pool.put("avgWaitTime", mbsc.getAttribute(poolBean, "avgWaitTime"));
            pool.put("avgInUseTime", mbsc.getAttribute(poolBean, "avgInUseTime"));
            pool.put("numConnectionsCreated", mbsc.getAttribute(poolBean, "numConnectionsCreated"));
            pool.put("numConnectionsDestroyed", mbsc.getAttribute(poolBean, "numConnectionsDestroyed"));
            
            pools.add(pool);
            count++;
        }
        
        return pools;
    }
    
    private static List<Map<String, Object>> collectJMSDestinations(Map<String, Object> command) throws Exception {
        List<Map<String, Object>> destinations = new ArrayList<>();
        int maxItems = getMaxItems(command);
        
        // WebSphere JMS destination MBeans
        ObjectName pattern = new ObjectName("WebSphere:type=JMSDestination,*");
        Set<ObjectName> destBeans = mbsc.queryNames(pattern, null);
        
        int count = 0;
        for (ObjectName destBean : destBeans) {
            if (maxItems > 0 && count >= maxItems) break;
            
            Map<String, Object> dest = new HashMap<>();
            dest.put("name", destBean.getKeyProperty("name"));
            dest.put("type", destBean.getKeyProperty("destinationType"));
            dest.put("messagesCurrentCount", mbsc.getAttribute(destBean, "messagesCurrentCount"));
            dest.put("messagesPendingCount", mbsc.getAttribute(destBean, "messagesPendingCount"));
            dest.put("messagesAddedCount", mbsc.getAttribute(destBean, "messagesAddedCount"));
            dest.put("consumerCount", mbsc.getAttribute(destBean, "consumerCount"));
            
            destinations.add(dest);
            count++;
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
        
        // WebSphere application MBeans
        ObjectName pattern = new ObjectName("WebSphere:type=Application,*");
        Set<ObjectName> appBeans = mbsc.queryNames(pattern, null);
        
        int count = 0;
        for (ObjectName appBean : appBeans) {
            if (maxItems > 0 && count >= maxItems) break;
            
            Map<String, Object> app = new HashMap<>();
            app.put("name", appBean.getKeyProperty("name"));
            app.put("requestCount", mbsc.getAttribute(appBean, "requestCount"));
            app.put("errorCount", mbsc.getAttribute(appBean, "errorCount"));
            app.put("avgResponseTime", mbsc.getAttribute(appBean, "avgResponseTime"));
            app.put("maxResponseTime", mbsc.getAttribute(appBean, "maxResponseTime"));
            
            if (collectSessions) {
                app.put("activeSessions", mbsc.getAttribute(appBean, "activeSessions"));
                app.put("liveSessions", mbsc.getAttribute(appBean, "liveSessions"));
                app.put("invalidatedSessions", mbsc.getAttribute(appBean, "invalidatedSessions"));
            }
            
            if (collectTransactions) {
                app.put("activeTransactions", mbsc.getAttribute(appBean, "activeTransactions"));
                app.put("committedTransactions", mbsc.getAttribute(appBean, "committedTransactions"));
                app.put("rolledbackTransactions", mbsc.getAttribute(appBean, "rolledbackTransactions"));
                app.put("timedoutTransactions", mbsc.getAttribute(appBean, "timedoutTransactions"));
            }
            
            applications.add(app);
            count++;
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
        } catch (Exception e) {
            sendError("Connection test failed", e.toString(), true);
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