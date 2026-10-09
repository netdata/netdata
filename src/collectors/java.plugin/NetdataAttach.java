// SPDX-License-Identifier: GPL-3.0-or-later

import com.sun.tools.attach.VirtualMachine;
import java.io.BufferedReader;
import java.io.InputStreamReader;
import java.io.InputStream;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.Path;
import java.util.Properties;

/** Fixed Attach API client, launched only after the native helper drops privileges. */
public final class NetdataAttach {
    private static String line(BufferedReader in) throws Exception {
        String value = in.readLine();
        if (value == null || value.length() > 256) throw new IllegalArgumentException();
        return value;
    }

    private static void identity(String pid, String start, String boot, String uid, String gid) throws Exception {
        Path proc = Path.of("/proc", pid);
        if (!Files.readString(Path.of("/proc/sys/kernel/random/boot_id")).trim().equals(boot)) {
            throw new IllegalStateException();
        }
        String stat = Files.readString(proc.resolve("stat"));
        if (!stat.substring(stat.lastIndexOf(") ") + 2).split(" ")[19].equals(start)) {
            throw new IllegalStateException();
        }
        boolean haveUid = false, haveGid = false;
        for (String value : Files.readAllLines(proc.resolve("status"))) {
            if (value.startsWith("Uid:") || value.startsWith("Gid:")) {
                String[] fields = value.trim().split("\\s+");
                String expected = value.startsWith("Uid:") ? uid : gid;
                if (fields.length != 5) throw new IllegalStateException();
                for (int i = 1; i < fields.length; i++) {
                    if (!fields[i].equals(expected)) throw new IllegalStateException();
                }
                if (value.startsWith("Uid:")) haveUid = true; else haveGid = true;
            }
        }
        if (!haveUid || !haveGid || uid.equals("0")) throw new IllegalStateException();
        if (!Files.readSymbolicLink(proc.resolve("exe")).getFileName().toString().equals("java")) {
            throw new IllegalStateException();
        }
        for (String arg : Files.readString(proc.resolve("cmdline")).split("\u0000")) {
            String lower = arg.toLowerCase(java.util.Locale.ROOT);
            if (lower.startsWith("-javaagent:") && (lower.contains("opentelemetry") || lower.contains("otel"))) {
                throw new IllegalStateException();
            }
        }
    }

    static void checkAttachArgument(String agent, String options) {
        // HotSpot's Attach listener limits each argument to 1024 bytes. The Java
        // Attach client passes the agent path and its options as one argument.
        if ((agent + "=" + options).getBytes(StandardCharsets.UTF_8).length > 1024) {
            throw new IllegalArgumentException();
        }
    }

    static String agentOptions(String pid, String start, String boot, String application,
            String token, String port, String directory) {
        String options = "otel.service.name=" + application
                + ";otel.resource.attributes=service.instance.id=" + boot + "-" + pid + ":" + start
                + ";otel.exporter.otlp.metrics.endpoint=http://127.0.0.1:" + port + "/v1/metrics"
                + ";otel.exporter.otlp.metrics.headers=x-netdata-java-token=" + token
                + ";otel.metrics.exporter=otlp;otel.traces.exporter=none;otel.logs.exporter=none"
                + ";otel.exporter.otlp.metrics.protocol=http/protobuf"
                + ";otel.metric.export.interval=1000"
                + ";otel.exporter.otlp.metrics.temporality.preference=cumulative"
                + ";otel.exporter.otlp.metrics.default.histogram.aggregation=explicit_bucket_histogram"
                + ";otel.javaagent.extensions=" + directory + "/hikari-extension.jar"
                + ";otel.javaagent.configuration-file=" + directory + "/otel.properties";
        checkAttachArgument(directory + "/otel.jar", options);
        return options;
    }

    static String readEnvironment(InputStream input) throws Exception {
        // Bound target-controlled input within the helper's 64 MiB heap budget.
        // Normal Linux exec argument/environment limits fit within this allowance.
        int limit = 8 * 1024 * 1024;
        byte[] bytes = input.readNBytes(limit + 1);
        if (bytes.length > limit) throw new IllegalArgumentException();
        return new String(bytes, StandardCharsets.ISO_8859_1);
    }

    static boolean compatibleConfiguration(Properties fixed, Properties system, String environment) {
        for (String key : fixed.stringPropertyNames()) {
            if (!key.startsWith("otel.instrumentation.")) continue;
            String expected = fixed.getProperty(key).trim();
            String actual = system.getProperty(key);
            if (actual == null) {
                String envKey = key.toUpperCase(java.util.Locale.ROOT).replace('.', '_').replace('-', '_') + "=";
                for (int offset = 0; offset < environment.length();) {
                    int end = environment.indexOf('\u0000', offset);
                    if (end < 0) end = environment.length();
                    if (offset + envKey.length() <= end && environment.startsWith(envKey, offset)) {
                        actual = environment.substring(offset + envKey.length(), end);
                        break;
                    }
                    offset = end + 1;
                }
            }
            if (actual != null && !actual.trim().equalsIgnoreCase(expected)) return false;
        }
        return true;
    }

    public static void main(String[] args) {
        boolean loading = false;
        try {
            if (args.length != 0) throw new IllegalArgumentException();
            BufferedReader in = new BufferedReader(new InputStreamReader(System.in, StandardCharsets.UTF_8));
            String pid = line(in), start = line(in), boot = line(in), uid = line(in), gid = line(in);
            String application = line(in), token = line(in), port = line(in), directory = line(in);
            if (!pid.matches("[1-9][0-9]*") || !start.matches("[1-9][0-9]*")
                    || !boot.matches("[a-f0-9-]{36}") || !uid.matches("[1-9][0-9]*")
                    || !gid.matches("[0-9]+") || !application.matches("[A-Za-z0-9._-]{1,128}")
                    || !token.matches("[a-fA-F0-9]{64}") || !port.matches("[0-9]+")
                    || Integer.parseInt(port) < 1024 || Integer.parseInt(port) > 65535
                    || !directory.matches("/[A-Za-z0-9._/-]+")) {
                throw new IllegalArgumentException();
            }
            String options = agentOptions(pid, start, boot, application, token, port, directory);
            identity(pid, start, boot, uid, gid);
            VirtualMachine vm = VirtualMachine.attach(pid);
            try {
                Properties properties = vm.getSystemProperties();
                Properties fixed = new Properties();
                try (InputStream config = Files.newInputStream(Path.of(directory, "otel.properties"))) {
                    fixed.load(config);
                }
                String environment;
                try (InputStream input = Files.newInputStream(Path.of("/proc", pid, "environ"))) {
                    environment = readEnvironment(input);
                }
                if (!compatibleConfiguration(fixed, properties, environment)) throw new IllegalStateException();
                int version = Integer.parseInt(properties.getProperty("java.specification.version", "0"));
                if (version < 17 || properties.containsKey("otel.javaagent.version")) {
                    throw new IllegalStateException();
                }
                identity(pid, start, boot, uid, gid);
                loading = true;
                vm.loadAgent(directory + "/otel.jar", options);
                System.out.println("ACK");
            } finally {
                vm.detach();
            }
        } catch (Throwable error) {
            // Exception messages may include agent options. Never print them or a stack trace.
            System.out.println(loading ? "UNKNOWN" : "BLOCKED");
        }
    }
}
