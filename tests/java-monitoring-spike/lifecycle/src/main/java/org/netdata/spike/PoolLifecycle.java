package org.netdata.spike;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.sun.net.httpserver.HttpExchange;
import com.sun.net.httpserver.HttpServer;
import com.zaxxer.hikari.HikariConfig;
import com.zaxxer.hikari.HikariDataSource;
import com.zaxxer.hikari.metrics.IMetricsTracker;
import com.zaxxer.hikari.metrics.MetricsTrackerFactory;
import java.lang.management.ManagementFactory;
import java.net.InetSocketAddress;
import java.sql.Connection;
import java.util.ArrayList;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.Future;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicLong;
import javax.management.ObjectName;

// Controlled oracle only. This is separate from the unchanged ordinary Spring application.
public class PoolLifecycle {
    private static final ObjectMapper JSON = new ObjectMapper();
    private final AtomicLong acquired = new AtomicLong();
    private final AtomicLong used = new AtomicLong();
    private final AtomicLong created = new AtomicLong();
    private final AtomicLong closed = new AtomicLong();
    private final MetricsTrackerFactory tracker = (name, stats) -> {
        created.incrementAndGet();
        return new IMetricsTracker() {
            @Override public void recordConnectionAcquiredNanos(long nanos) { acquired.incrementAndGet(); }
            @Override public void recordConnectionUsageMillis(long millis) { used.incrementAndGet(); }
            @Override public void close() { closed.incrementAndGet(); }
        };
    };
    private volatile HikariDataSource pool = create("controlled", 2, false);
    private final HikariDataSource idle = create("idle-until-used", 1, false);
    private final List<Connection> held = new ArrayList<>();
    private final ExecutorService executor = Executors.newFixedThreadPool(24);
    private Future<?> waiter;

    private HikariDataSource create(String name, int limit, boolean lazy) {
        HikariConfig config = lazy ? new HikariDataSource() : new HikariConfig();
        config.setPoolName(name);
        config.setJdbcUrl("jdbc:h2:mem:" + name);
        config.setMaximumPoolSize(limit);
        config.setMinimumIdle(limit);
        config.setConnectionTimeout(60000);
        config.setMetricsTrackerFactory(tracker);
        return lazy ? (HikariDataSource) config : new HikariDataSource(config);
    }

    private Map<String, Object> state() throws Exception {
        var bean = pool.getHikariPoolMXBean();
        Map<String, Object> values = new LinkedHashMap<>();
        values.put("active", bean == null ? null : bean.getActiveConnections());
        values.put("idle", bean == null ? null : bean.getIdleConnections());
        values.put("pending", bean == null ? null : bean.getThreadsAwaitingConnection());
        values.put("limit", pool.getMaximumPoolSize());
        values.put("closed", pool.isClosed());
        values.put("tracker_preserved", pool.getMetricsTrackerFactory() == tracker);
        values.put("tracker_acquired", acquired.get());
        values.put("tracker_used", used.get());
        values.put("trackers_created", created.get());
        values.put("trackers_closed", closed.get());
        values.put("hikari_mbeans", ManagementFactory.getPlatformMBeanServer()
                .queryNames(new ObjectName("com.zaxxer.hikari:*"), null).size());
        return values;
    }

    private void handle(HttpExchange exchange) {
        try {
            String path = exchange.getRequestURI().getPath();
            boolean readOnly = path.equals("/health") || path.equals("/state");
            if (!readOnly && !exchange.getRequestMethod().equals("POST")) {
                exchange.sendResponseHeaders(405, -1);
                return;
            }
            switch (path) {
                case "/health", "/state" -> { }
                case "/borrow" -> {
                    try (Connection connection = pool.getConnection()) {
                        connection.isValid(1);
                    }
                }
                case "/idle-use" -> {
                    try (Connection connection = idle.getConnection()) {
                        connection.isValid(1);
                    }
                }
                case "/hold" -> {
                    for (int i = 0; i < pool.getMaximumPoolSize(); i++) {
                        held.add(pool.getConnection());
                    }
                    waiter = executor.submit(() -> {
                        try (Connection connection = pool.getConnection()) {
                            connection.isValid(1);
                        } catch (Exception error) {
                            throw new IllegalStateException(error);
                        }
                    });
                }
                case "/release" -> {
                    for (Connection connection : held) { connection.close(); }
                    held.clear();
                    if (waiter != null) { waiter.get(5, TimeUnit.SECONDS); waiter = null; }
                }
                case "/close" -> pool.close();
                case "/recreate" -> {
                    if (!pool.isClosed()) { throw new IllegalStateException("close the previous pool first"); }
                    pool = create("controlled", 3, true);
                }
                case "/idle-close" -> idle.close();
                default -> throw new IllegalArgumentException("unknown fixture operation");
            }
            byte[] response = JSON.writeValueAsBytes(state());
            exchange.getResponseHeaders().set("Content-Type", "application/json");
            exchange.sendResponseHeaders(200, response.length);
            exchange.getResponseBody().write(response);
        } catch (Exception error) {
            try {
                byte[] response = JSON.writeValueAsBytes(Map.of("error", error.toString()));
                exchange.sendResponseHeaders(500, response.length);
                exchange.getResponseBody().write(response);
            } catch (Exception ignored) { }
        } finally {
            exchange.close();
        }
    }

    public static void main(String[] args) throws Exception {
        PoolLifecycle fixture = new PoolLifecycle();
        // Both pools are created and used before the external harness can attach an agent.
        try (Connection first = fixture.pool.getConnection(); Connection second = fixture.idle.getConnection()) {
            first.isValid(1); second.isValid(1);
        }
        HttpServer server = HttpServer.create(new InetSocketAddress(8080), 0);
        server.createContext("/", fixture::handle);
        server.setExecutor(fixture.executor);
        server.start();
    }
}
