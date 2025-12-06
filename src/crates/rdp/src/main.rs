use clap::Parser;
use rdp::{encode, encode_full, has_checksum, verify_structure_round_trip};
use std::time::Instant;

#[derive(Parser, Debug)]
#[command(name = "rdp")]
#[command(about = "RDP - String tokenizer, parser, and encoder for field names", long_about = None)]
struct Args {
    /// Run comprehensive fuzz testing on all valid strings up to the specified max length.
    /// If no value is provided, defaults to 4.
    #[arg(long, value_name = "MAX_LENGTH", default_missing_value = "4", num_args = 0..=1)]
    fuzz: Option<usize>,
}

fn main() {
    let args = Args::parse();

    if let Some(max_length) = args.fuzz {
        run_fuzz_tests(max_length);
    } else {
        run_examples();
    }
}

fn run_examples() {
    let examples = vec![
        // Simple cases
        ("hello", "Simple lowercase"),
        ("HELLO", "Simple uppercase"),
        ("Hello", "Simple capitalized"),
        // Camel case
        ("helloWorld", "Lower camel case"),
        ("HelloWorld", "Upper camel case"),
        ("myVarName", "Lower camel case (multi-word)"),
        // With separators
        ("hello.world", "Dot separator"),
        ("hello_world", "Underscore separator"),
        ("hello-world", "Hyphen separator"),
        // Complex real-world examples
        ("log.body.HostName", "Complex with camel case"),
        ("log.body.hostname", "Complex without camel case"),
        ("parseHTMLString", "Mixed case types"),
        // Edge case that would collide without checksum
        ("log.body.HostName", "Collision test 1"),
        ("log.body.HostnAme", "Collision test 2"),
    ];

    println!("Key Encoding Examples");
    println!("====================\n");

    for (key, description) in examples {
        let encoded = encode(key);
        let with_checksum = has_checksum(&encoded);

        println!(
            "{:30} → {:10} (checksum: {})",
            key,
            format!("\"{}\"", encoded),
            with_checksum
        );
        println!("  Description: {}", description);
        println!(
            "  Compression: {} bytes → {} bytes",
            key.len(),
            encoded.len()
        );
        println!();
    }

    let aws_keys = vec![
        "log.attributes.log.file.path",
        "log.attributes.log.iostream",
        "log.attributes.logtag",
        "log.dropped_attributes_count",
        "log.event_name",
        "log.flags",
        "log.observed_time_unix_nano",
        "log.severity_number",
        "log.severity_text",
        "log.time_unix_nano",
        "resource.attributes.host.name",
        "resource.attributes.k8s.container.name",
        "resource.attributes.k8s.container.restart_count",
        "resource.attributes.k8s.namespace.name",
        "resource.attributes.k8s.node.name",
        "resource.attributes.k8s.pod.name",
        "resource.attributes.k8s.pod.start_time",
        "resource.attributes.k8s.pod.uid",
        "resource.attributes.os.type",
        "resource.schema_url",
        "resource.attributes.k8s.deployment.name",
        "log.body.level",
        "log.body.message",
        "log.body.time",
        "log.body.severity",
        "log.body.timestamp",
        "log.body.local_addr.IP",
        "log.body.local_addr.Port",
        "log.body.local_addr.Zone",
        "log.body.remote_addr.ForceQuery",
        "log.body.remote_addr.Fragment",
        "log.body.remote_addr.Host",
        "log.body.remote_addr.OmitHost",
        "log.body.remote_addr.Opaque",
        "log.body.remote_addr.Path",
        "log.body.remote_addr.RawFragment",
        "log.body.remote_addr.RawPath",
        "log.body.remote_addr.RawQuery",
        "log.body.remote_addr.Scheme",
        "log.body.remote_addr.User",
        "log.body.file",
        "log.body.id",
        "log.body.line",
        "log.body.TR",
        "log.body.Ta",
        "log.body.Tc",
        "log.body.Th",
        "log.body.Ti",
        "log.body.Tr",
        "log.body.Tw",
        "log.body.actconn",
        "log.body.backend_name",
        "log.body.backend_queue",
        "log.body.beconn",
        "log.body.bytes_read",
        "log.body.cf_conn_ip",
        "log.body.client_ip",
        "log.body.client_port",
        "log.body.date_time",
        "log.body.feconn",
        "log.body.frontend_name",
        "log.body.http_method",
        "log.body.http_query",
        "log.body.http_uri",
        "log.body.http_version",
        "log.body.retries",
        "log.body.server_name",
        "log.body.srv_queue",
        "log.body.srvconn",
        "log.body.ssl_ciphers",
        "log.body.ssl_version",
        "log.body.status_code",
        "log.body.termination_state",
        "log.body.unique_id",
        "log.body.size",
        "log.body.status",
        "resource.attributes.k8s.daemonset.name",
        "resource.attributes.component",
        "log.body.fields.otelcol.component.id",
        "log.body.fields.otelcol.component.kind",
        "log.body.fields.otelcol.signal",
        "log.body.fields.resource.service.instance.id",
        "log.body.fields.resource.service.name",
        "log.body.fields.resource.service.version",
        "log.body.fields.data",
        "log.body.fields.metrics",
        "log.body.fields.resource",
        "log.body.topic",
        "log.body.msg",
        "log.body.method",
        "log.body.path",
        "log.body.agent",
        "log.body.host",
        "log.body.protocol",
        "log.body.referrer",
        "log.body.user_id",
        "log.body.duration",
        "log.body.ts",
        "log.body.logger",
        "log.body.request.host",
        "log.body.request.method",
        "log.body.request.proto",
        "log.body.request.remote_ip",
        "log.body.request.remote_port",
        "log.body.request.uri",
        "log.body.serviceURL.ForceQuery",
        "log.body.serviceURL.Fragment",
        "log.body.serviceURL.Host",
        "log.body.serviceURL.OmitHost",
        "log.body.serviceURL.Opaque",
        "log.body.serviceURL.Path",
        "log.body.serviceURL.RawFragment",
        "log.body.serviceURL.RawPath",
        "log.body.serviceURL.RawQuery",
        "log.body.serviceURL.Scheme",
        "log.body.serviceURL.User",
        "log.body.ClientAddr",
        "log.body.ClientHost",
        "log.body.ClientPort",
        "log.body.ClientUsername",
        "log.body.DownstreamContentSize",
        "log.body.DownstreamStatus",
        "log.body.Duration",
        "log.body.GzipRatio",
        "log.body.OriginContentSize",
        "log.body.OriginDuration",
        "log.body.OriginStatus",
        "log.body.Overhead",
        "log.body.RequestAddr",
        "log.body.RequestContentSize",
        "log.body.RequestCount",
        "log.body.RequestHost",
        "log.body.RequestMethod",
        "log.body.RequestPath",
        "log.body.RequestPort",
        "log.body.RequestProtocol",
        "log.body.RequestScheme",
        "log.body.RetryAttempts",
        "log.body.RouterName",
        "log.body.ServiceAddr",
        "log.body.ServiceName",
        "log.body.ServiceURL",
        "log.body.StartLocal",
        "log.body.StartUTC",
        "log.body.entryPointName",
        "log.body.request_User-Agent",
        "resource.attributes.app",
        "log.body.apiVersion",
        "log.body.eventTime",
        "log.body.firstTimestamp",
        "log.body.involvedObject.apiVersion",
        "log.body.involvedObject.kind",
        "log.body.involvedObject.name",
        "log.body.involvedObject.resourceVersion",
        "log.body.involvedObject.uid",
        "log.body.kind",
        "log.body.lastTimestamp",
        "log.body.metadata.creationTimestamp",
        "log.body.metadata.name",
        "log.body.metadata.namespace",
        "log.body.metadata.resourceVersion",
        "log.body.metadata.uid",
        "log.body.reason",
        "log.body.reportingComponent",
        "log.body.reportingInstance",
        "log.body.type",
        "log.body.involvedObject.namespace",
        "log.body.count",
        "log.body.source.component",
        "log.body.record-id",
        "log.body.record-type",
        "log.body.service",
        "log.body.bootstrap",
        "log.body.config_file_source",
        "log.body.session_id",
        "log.body.source.host",
        "log.body.involvedObject.fieldPath",
        "log.body.authority",
        "log.body.forwarded-for",
        "log.body.referer",
        "log.body.request-id",
        "log.body.response-code",
        "log.body.response-code-details",
        "log.body.upstream-cluster",
        "log.body.user-agent",
        "log.body.svc",
        "resource.attributes.k8s.cronjob.name",
        "resource.attributes.k8s.job.name",
        "log.body.account",
        "log.body.action",
        "log.body.arn",
        "log.body.caller",
        "log.body.err",
        "log.body.job_type",
        "log.body.region",
        "log.body.version",
        "log.body.fields.component",
        "log.body.fields.path",
        "log.body.error",
        "log.body.agents_missing_from_emqx",
        "log.body.agents_unreachable_postgres_disconnected",
        "log.body.emqx_client_id_count",
        "log.body.failed",
        "log.body.fields.error",
        "log.body.fields.interval",
        "log.body.fields_raw",
        "log.body.progress",
        "log.body.successful",
        "log.body.unreachable_agents_cleaned",
        "my.deeply.nested.key.checks.run.length.encoding.works.well",
        "log.body.HOSTNAME",
        "HTTPSConnection",
        "OAuth2Token",
        "foo.bar",
        "foo_bar",
        "fooBar",
        "resource.attributes.service.instance.environment.region.zone",
    ];

    println!("Otel,Otel key length,Systemd key,Systemd key length, Length diff");
    for otel_key in aws_keys {
        let systemd_key = encode_full(otel_key.as_bytes());
        println!("{} {}", otel_key, systemd_key);
    }
}

fn run_fuzz_tests(max_length: usize) {
    // Character set: a-z, A-Z, 0-9, . _ -
    const CHARSET: &[u8] = b"abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789._-";
    const CHARSET_LEN: usize = CHARSET.len();

    // Calculate total number of tests based on max_length
    // Length 0: 1 (empty string)
    // Length n: CHARSET_LEN^n
    let mut total_tests: usize = 1; // empty string
    for length in 1..=max_length {
        total_tests += CHARSET_LEN.pow(length as u32);
    }

    println!("Starting comprehensive fuzz testing...");
    println!("Character set: [a-zA-Z0-9._-] ({} characters)", CHARSET_LEN);
    println!("Testing all strings with length 0-{}", max_length);
    println!("Total tests: {}\n", total_tests);

    let start_time = Instant::now();
    let mut tested = 0;
    let mut failed = 0;
    let report_interval = 1_000_000;
    let mut next_report = report_interval;

    // Test empty string
    test_string("", &mut tested, &mut failed);

    // Generate and test all strings of length 1..=max_length
    for length in 1..=max_length {
        generate_strings(
            length,
            CHARSET,
            &mut tested,
            &mut failed,
            &mut next_report,
            report_interval,
            total_tests,
        );
    }

    let elapsed = start_time.elapsed();
    let tests_per_sec = tested as f64 / elapsed.as_secs_f64();

    println!("\n{}", "=".repeat(70));
    println!("Fuzz Testing Complete!");
    println!("{}", "=".repeat(70));
    println!("Total tests run: {}", tested);
    println!("Tests passed:    {}", tested - failed);
    println!("Tests failed:    {}", failed);
    println!("Time elapsed:    {:.2}s", elapsed.as_secs_f64());
    println!("Tests per sec:   {:.0}", tests_per_sec);

    if failed > 0 {
        println!("\n⚠️  WARNING: {} test(s) failed!", failed);
        std::process::exit(1);
    } else {
        println!("\n✅ All tests passed!");
    }
}

fn generate_strings(
    length: usize,
    charset: &[u8],
    tested: &mut usize,
    failed: &mut usize,
    next_report: &mut usize,
    report_interval: usize,
    total_tests: usize,
) {
    let mut indices = vec![0usize; length];
    let charset_len = charset.len();

    loop {
        // Build the current string
        let s: String = indices.iter().map(|&i| charset[i] as char).collect();

        // Test it
        test_string(&s, tested, failed);

        // Report progress
        if *tested >= *next_report {
            let percent = (*tested as f64 / total_tests as f64) * 100.0;
            println!(
                "Progress: {:>10} tests completed ({:>6.2}%, {} failed)",
                tested, percent, failed
            );
            *next_report += report_interval;
        }

        // Increment to next combination
        let mut pos = length - 1;
        loop {
            indices[pos] += 1;
            if indices[pos] < charset_len {
                break;
            }
            if pos == 0 {
                // We've exhausted all combinations for this length
                return;
            }
            indices[pos] = 0;
            pos -= 1;
        }
    }
}

fn test_string(s: &str, tested: &mut usize, failed: &mut usize) {
    *tested += 1;

    if let Err(err) = verify_structure_round_trip(s) {
        eprintln!("\n❌ Test failed: {}", err);
        *failed += 1;
    }
}
