package org.netdata.spike.hikari;

import com.zaxxer.hikari.HikariDataSource;
import com.zaxxer.hikari.HikariPoolMXBean;
import io.opentelemetry.api.GlobalOpenTelemetry;
import io.opentelemetry.api.common.Attributes;
import io.opentelemetry.api.metrics.BatchCallback;
import io.opentelemetry.api.metrics.Meter;
import io.opentelemetry.api.metrics.ObservableLongMeasurement;
import java.lang.ref.WeakReference;
import java.util.Map;
import java.util.WeakHashMap;
import java.util.concurrent.atomic.AtomicLong;

public final class PoolObserver {
    private static final int MAX_POOLS = 128;
    private static final Map<HikariDataSource, Registration> POOLS = new WeakHashMap<>();
    private static final AtomicLong IDS = new AtomicLong();
    private static final Meter METER = GlobalOpenTelemetry.getMeter("org.netdata.spike.hikari");
    private static final ObservableLongMeasurement USAGE = METER.gaugeBuilder("netdata.spike.hikari.connections")
            .setUnit("{connection}").ofLongs().buildObserver();
    private static final ObservableLongMeasurement PENDING = METER.gaugeBuilder("netdata.spike.hikari.pending_requests")
            .setUnit("{request}").ofLongs().buildObserver();
    private static final ObservableLongMeasurement LIMIT = METER.gaugeBuilder("netdata.spike.hikari.limit")
            .setUnit("{connection}").ofLongs().buildObserver();

    private PoolObserver() {}

    public static boolean observe(HikariDataSource source) {
        synchronized (POOLS) {
            if (POOLS.containsKey(source)) {
                return true;
            }
            if (source.isClosed() || source.getHikariPoolMXBean() == null || POOLS.size() >= MAX_POOLS) {
                return false;
            }
            Registration registration = new Registration(source, IDS.incrementAndGet());
            // A collection may start as soon as batchCallback publishes the observer.
            synchronized (registration) {
                registration.callback = METER.batchCallback(registration, USAGE, PENDING, LIMIT);
            }
            POOLS.put(source, registration);
            return true;
        }
    }

    public static void remove(HikariDataSource source) {
        Registration registration;
        synchronized (POOLS) {
            registration = POOLS.remove(source);
        }
        if (registration != null) {
            registration.close();
        }
    }

    public static final class Registration implements Runnable {
        private final WeakReference<HikariDataSource> source;
        private final Attributes attributes;
        private final Attributes active;
        private final Attributes idle;
        private BatchCallback callback;
        private boolean closed;

        Registration(HikariDataSource source, long id) {
            this.source = new WeakReference<>(source);
            attributes = Attributes.builder().put("pool.name", source.getPoolName())
                    .put("pool.id", Long.toString(id)).build();
            active = attributes.toBuilder().put("state", "active").build();
            idle = attributes.toBuilder().put("state", "idle").build();
        }

        @Override
        public synchronized void run() {
            if (closed) {
                return;
            }
            HikariDataSource target = source.get();
            if (target == null || target.isClosed()) {
                close();
                return;
            }
            HikariPoolMXBean pool = target.getHikariPoolMXBean();
            if (pool != null) {
                USAGE.record(pool.getActiveConnections(), active);
                USAGE.record(pool.getIdleConnections(), idle);
                PENDING.record(pool.getThreadsAwaitingConnection(), attributes);
                LIMIT.record(target.getMaximumPoolSize(), attributes);
            }
        }

        synchronized void close() {
            if (!closed) {
                closed = true;
                if (callback != null) {
                    callback.close();
                }
            }
        }
    }
}
