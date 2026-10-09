// SPDX-License-Identifier: GPL-3.0-or-later

import com.sun.tools.attach.VirtualMachine;
import java.io.BufferedReader;
import java.io.InputStreamReader;
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
            identity(pid, start, boot, uid, gid);
            VirtualMachine vm = VirtualMachine.attach(pid);
            try {
                Properties properties = vm.getSystemProperties();
                int version = Integer.parseInt(properties.getProperty("java.specification.version", "0"));
                if (version < 17 || properties.containsKey("otel.javaagent.version")) {
                    throw new IllegalStateException();
                }
                identity(pid, start, boot, uid, gid);
                String options = "otel.service.name=" + application
                        + ";otel.resource.attributes=service.instance.id=" + boot + "-" + pid + ":" + start
                        + ";otel.exporter.otlp.endpoint=http://127.0.0.1:" + port
                        + ";otel.exporter.otlp.protocol=http/protobuf"
                        + ";otel.exporter.otlp.headers=x-netdata-java-token=" + token
                        + ";otel.metrics.exporter=otlp;otel.traces.exporter=none;otel.logs.exporter=none"
                        + ";otel.metric.export.interval=1000"
                        + ";otel.exporter.otlp.metrics.temporality.preference=cumulative"
                        + ";otel.exporter.otlp.metrics.default.histogram.aggregation=explicit_bucket_histogram"
                        + ";otel.javaagent.extensions=" + directory + "/hikari-extension.jar"
                        + ";otel.instrumentation.hikaricp.enabled=false"
                        + ";otel.instrumentation.common.default-enabled=false"
                        + ";otel.instrumentation.runtime-telemetry.enabled=true"
                        + ";otel.instrumentation.servlet.enabled=true"
                        + ";otel.instrumentation.tomcat.enabled=true"
                        + ";otel.instrumentation.spring-webmvc.enabled=true"
                        + ";otel.instrumentation.netdata-hikari.enabled=true";
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
