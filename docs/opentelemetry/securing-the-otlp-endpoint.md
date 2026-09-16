# Securing the OTLP Endpoint

The OTLP endpoint accepts whatever reaches it, so its security is the transport: where it listens, TLS, and network
controls. Everything below is set in `otel.yaml` (edit it with
[`edit-config`](/docs/netdata-agent/configuration/README.md#edit-configuration-files)) and applied by restarting the
Agent — including certificate replacements.

## Keep the default when you can

The plugin listens on `127.0.0.1:4317` by default: only processes on the same host can reach it, and TLS is
unnecessary for the network path. Loopback limits reach, not identity: any local process can send records and, with
tenants enabled, select any tenant. Keep the default on hosts where every local process is trusted, such as a node
that runs one Collector forwarding to its local Agent. On a shared host, enable TLS with client certificates on the
loopback listener as described below, or restrict which local users may connect to the port with the host firewall
(netfilter's `owner` match).

## Accepting remote senders

Bind beyond loopback only with TLS, and prefer mutual TLS:

```yaml
endpoint:
  path: "0.0.0.0:4317"
  tls_cert_path: /etc/netdata/ssl/server-cert.pem
  tls_key_path: /etc/netdata/ssl/server-key.pem
  # Require client certificates (mutual TLS): senders must present a
  # certificate signed by this CA.
  tls_ca_cert_path: /etc/netdata/ssl/client-ca.pem
```

- Never expose a plaintext listener beyond loopback.
- Restrict port 4317 with network access controls (firewall, security groups) to the senders' addresses; the endpoint
  speaks OTLP/gRPC only.
- Issue the server certificate from whatever your infrastructure already trusts — an internal CA or your certificate
  automation; the senders configure the matching `ca_file` (and, for mutual TLS, their client certificate and key) as
  shown in [Collect Logs with OpenTelemetry Collector](/docs/opentelemetry/logs-collection.md#shared-exporter).
- After rotating certificates, restart the Netdata Agent to load the new files.

## Tenants are selection, not authentication

```yaml
auth:
  enabled: true
```

With tenant selection enabled, every log and trace sender must set the `X-Scope-OrgID` header; requests without it
are rejected. Metrics are not tenant-scoped. The header chooses the tenant — its storage tree and its retention
policy — and nothing more. It does not authenticate
the sender: any client that passes TLS can claim any tenant, so rely on mutual TLS and network controls to decide who
can send at all, and treat tenants as an organization tool (one retention policy per team, environment, or system).
See [Log Storage and Retention](/docs/logs/log-storage-and-retention.md) for per-tenant retention.

## What reaches Netdata Cloud

Received telemetry is stored on the Agent, not in Netdata Cloud. Viewing logs requires a signed-in Netdata Cloud user
of the Agent's Space; when viewing through Netdata Cloud, content is transmitted encrypted to the browser and is not
stored in Netdata Cloud.

## Checklist

- [ ] Endpoint bound only where senders need it; plaintext only on loopback.
- [ ] TLS server certificate and key in place; mutual TLS where the network is not trusted.
- [ ] Port 4317 restricted to known sender addresses.
- [ ] Tenant selection enabled when different sender groups need separate retention.
- [ ] A restart procedure for certificate rotation.
