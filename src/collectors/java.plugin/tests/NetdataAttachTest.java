// SPDX-License-Identifier: GPL-3.0-or-later

import java.nio.charset.StandardCharsets;
import java.io.ByteArrayInputStream;
import java.util.Properties;

/** Standalone regression checks; requires no target JVM or attachment permission. */
public final class NetdataAttachTest {
    private static void require(boolean condition) {
        if (!condition) throw new AssertionError();
    }

    private static void rejected(Runnable operation) {
        try {
            operation.run();
        } catch (IllegalArgumentException expected) {
            return;
        }
        throw new AssertionError("Oversized Attach argument was accepted");
    }

    public static void main(String[] args) throws Exception {
        String directory = "/opt/netdata-java-test-123456789012/share/netdata/java";
        String token = "a".repeat(64);
        String options = NetdataAttach.agentOptions("4194304", "123456789012",
                "11111111-1111-1111-1111-111111111111", "n".repeat(128), token, "65535", directory);
        require(options.contains("otel.javaagent.configuration-file=" + directory + "/otel.properties"));
        require(options.contains("otel.exporter.otlp.metrics.headers=x-netdata-java-token=" + token));
        require(options.contains("otel.exporter.otlp.metrics.endpoint=http://127.0.0.1:65535/v1/metrics"));
        // All theoretical scalar maxima together exceed the protocol even at
        // this prefix; reject clearly instead of launching an ambiguous load.
        rejected(() -> NetdataAttach.agentOptions("2147483647", "18446744073709551615",
                "11111111-1111-1111-1111-111111111111", "n".repeat(128), token, "65535", directory));
        require(!options.contains("otel.instrumentation."));
        require(options.contains("otel.javaagent.extensions=" + directory + "/hikari-extension.jar"));
        for (String forced : new String[] {"otel.metrics.exporter=otlp", "otel.traces.exporter=none",
                "otel.logs.exporter=none", "otel.exporter.otlp.metrics.protocol=http/protobuf",
                "otel.metric.export.interval=1000", "otel.exporter.otlp.metrics.temporality.preference=cumulative",
                "otel.exporter.otlp.metrics.default.histogram.aggregation=explicit_bucket_histogram"}) {
            require(options.contains(forced));
        }
        require((directory + "/otel.jar=" + options).getBytes(StandardCharsets.UTF_8).length <= 1024);

        String agent = "/otel.jar";
        int available = 1024 - (agent + "=").getBytes(StandardCharsets.UTF_8).length;
        NetdataAttach.checkAttachArgument(agent, "a".repeat(available));
        rejected(() -> NetdataAttach.checkAttachArgument(agent, "a".repeat(available + 1)));
        // The protocol counts encoded bytes, not Java characters.
        rejected(() -> NetdataAttach.checkAttachArgument(agent, "é".repeat(available / 2 + 1)));
        rejected(() -> NetdataAttach.agentOptions("42", "123", "11111111-1111-1111-1111-111111111111",
                "orders", token, "65535", "/" + "a".repeat(600)));
        Properties fixed = new Properties();
        fixed.setProperty("otel.instrumentation.common.default-enabled", "false");
        fixed.setProperty("otel.instrumentation.runtime-telemetry.enabled", "true");
        Properties system = new Properties();
        require(NetdataAttach.compatibleConfiguration(fixed, system, "UNRELATED=value\u0000"));
        require(NetdataAttach.compatibleConfiguration(fixed, system,
                "OTEL_INSTRUMENTATION_RUNTIME_TELEMETRY_ENABLED= TRUE \u0000"));
        require(!NetdataAttach.compatibleConfiguration(fixed, system,
                "OTEL_INSTRUMENTATION_RUNTIME_TELEMETRY_ENABLED=false\u0000"));
        system.setProperty("otel.instrumentation.common.default-enabled", "true");
        require(!NetdataAttach.compatibleConfiguration(fixed, system, ""));
        system.setProperty("otel.instrumentation.common.default-enabled", " FALSE ");
        // A matching system property has precedence over a conflicting environment value.
        require(NetdataAttach.compatibleConfiguration(fixed, system,
                "OTEL_INSTRUMENTATION_COMMON_DEFAULT_ENABLED=true\u0000"));
        String environment = "OTEL_INSTRUMENTATION_RUNTIME_TELEMETRY_ENABLED=true\u0000";
        require(NetdataAttach.readEnvironment(new ByteArrayInputStream(
                environment.getBytes(StandardCharsets.ISO_8859_1))).equals(environment));
        try {
            NetdataAttach.readEnvironment(new ByteArrayInputStream(new byte[8 * 1024 * 1024 + 1]));
            throw new AssertionError("Oversized environment was accepted");
        } catch (IllegalArgumentException expected) {
            // No target-controlled values are included in diagnostics.
        }
        System.out.println("Attach argument and configuration boundary checks passed");
    }
}
