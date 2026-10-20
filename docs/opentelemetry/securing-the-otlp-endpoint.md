# Securing the OTLP Endpoint

The OTLP endpoint accepts whatever reaches it, so its security is the transport: where it listens, TLS, and network
controls. Everything below is set in `otel.yaml` (edit it with
[`edit-config`](/docs/netdata-agent/configuration/README.md#edit-configuration-files)) and applied by restarting the
Agent — including certificate replacements.

## Keep the default when you can

The plugin listens on `127.0.0.1:4317` (OTLP/gRPC) by default. The OTLP/HTTP listener, on `127.0.0.1:4318`, is off until
you turn it on. Only processes on the same host can reach these loopback addresses, and TLS is unnecessary for the
network path. Loopback limits reach, not identity: any local process can send records. Keep the defaults on hosts where
every local process is trusted, such as a node that runs one Collector forwarding to its local Agent. On a shared host,
enable TLS with client certificates on the loopback listeners as described below, or restrict which local users may
connect to the ports with the host firewall (netfilter's `owner` match).

## Accepting remote senders

Bind beyond loopback only with TLS, and prefer mutual TLS:

```yaml
receivers:
  otlp:
    protocols:
      grpc:
        endpoint: "0.0.0.0:4317"
        tls:
          cert_file: /etc/netdata/ssl/server-cert.pem
          key_file: /etc/netdata/ssl/server-key.pem
          # Require client certificates (mutual TLS): senders must present a
          # certificate signed by this CA.
          client_ca_file: /etc/netdata/ssl/client-ca.pem
      # The OTLP/HTTP listener has its own address and its own TLS settings.
      http:
        enabled: true
        endpoint: "0.0.0.0:4318"
        tls:
          cert_file: /etc/netdata/ssl/server-cert.pem
          key_file: /etc/netdata/ssl/server-key.pem
          client_ca_file: /etc/netdata/ssl/client-ca.pem
```

- Never expose a plaintext listener beyond loopback.
- Restrict ports `4317` (OTLP/gRPC) and `4318` (OTLP/HTTP) with network access controls (firewall, security groups) to
  the senders' addresses. If no sender uses one of the listeners, turn it off (`enabled: false` under `grpc` or `http`)
  instead of leaving it exposed. At least one listener must stay on.
- Issue the server certificate from whatever your infrastructure already trusts — an internal CA or your certificate
  automation; the senders configure the matching `ca_file` (and, for mutual TLS, their client certificate and key) as
  shown in [Collect Logs with OpenTelemetry Collector](/docs/opentelemetry/logs-collection.md#shared-exporter).
- After rotating certificates, restart the Netdata Agent to load the new files.

## What reaches Netdata Cloud

Received telemetry is stored on the Agent, not in Netdata Cloud. Viewing logs or traces requires a signed-in Netdata
Cloud user of the Agent's Space; when viewing through Netdata Cloud, content is transmitted encrypted to the browser
and is not stored in Netdata Cloud.

## Checklist

- [ ] Endpoint bound only where senders need it; plaintext only on loopback.
- [ ] TLS server certificate and key in place; mutual TLS where the network is not trusted.
- [ ] Ports 4317 (OTLP/gRPC) and 4318 (OTLP/HTTP) restricted to known sender addresses, and an unused listener
      turned off (`enabled: false`).
- [ ] A restart procedure for certificate rotation.
