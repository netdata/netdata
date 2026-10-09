import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.time.Duration;
import java.util.Arrays;
import java.util.concurrent.Executors;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicInteger;
import java.util.concurrent.locks.LockSupport;

/** Internal, paced fixed-count workload: avoids Docker Desktop's host port forwarding. */
public class HttpLoad {
    public static void main(String[] args) throws Exception {
        String url = args[0];
        int seconds = Integer.parseInt(args[1]);
        int rate = Integer.parseInt(args[2]);
        int count = seconds * rate;
        var client = HttpClient.newBuilder().connectTimeout(Duration.ofSeconds(3)).build();
        var executor = Executors.newFixedThreadPool(16);
        long start = System.nanoTime();
        long[] latency = new long[count];
        long[] lag = new long[count];
        AtomicInteger ok = new AtomicInteger(), failed = new AtomicInteger(), errors = new AtomicInteger();
        for (int index = 0; index < count; index++) {
            final int i = index;
            long scheduled = start + i * 1_000_000_000L / rate;
            long delay;
            while ((delay = scheduled - System.nanoTime()) > 0) LockSupport.parkNanos(delay);
            executor.submit(() -> {
                long before = System.nanoTime();
                lag[i] = before - scheduled;
                try {
                    int status = client.send(HttpRequest.newBuilder(URI.create(url + "/work?holdMs=0&fail="
                            + (i % 10 == 0))).timeout(Duration.ofSeconds(5)).build(),
                            HttpResponse.BodyHandlers.discarding()).statusCode();
                    if (status == 200) ok.incrementAndGet();
                    else if (status == 503) failed.incrementAndGet();
                    else errors.incrementAndGet();
                } catch (Exception error) {
                    errors.incrementAndGet();
                }
                latency[i] = System.nanoTime() - before;
            });
        }
        executor.shutdown();
        if (!executor.awaitTermination(60, TimeUnit.SECONDS)) throw new IllegalStateException("load drain timeout");
        double elapsed = (System.nanoTime() - start) / 1e9;
        Arrays.sort(latency);
        Arrays.sort(lag);
        System.out.printf(java.util.Locale.ROOT,
                "{\"requests\":%d,\"statuses\":{\"200\":%d,\"503\":%d},\"errors\":%d,\"seconds\":%.3f,"
                + "\"p50_ms\":%.3f,\"p95_ms\":%.3f,\"schedule_lag_p95_ms\":%.3f}%n",
                count, ok.get(), failed.get(), errors.get(), elapsed,
                latency[count / 2] / 1e6, latency[count * 95 / 100] / 1e6, lag[count * 95 / 100] / 1e6);
        if (errors.get() != 0 || ok.get() != count - count / 10 || failed.get() != count / 10) {
            throw new IllegalStateException("unexpected workload response");
        }
    }
}
