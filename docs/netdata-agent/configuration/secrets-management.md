# Secrets Management

## Overview

Netdata collectors often need credentials such as database passwords, API tokens, and connection strings. Instead of storing these credentials as plain text in configuration files, you can use **secret references** that are resolved automatically when a collector job starts.

Secret references work with both YAML configuration files and the [Dynamic Configuration Manager](/docs/netdata-agent/configuration/dynamic-configuration.md). Collectors require zero changes — resolution is fully transparent.

:::tip

**TL;DR** — Replace any plain-text secret in a collector config with a reference like `${env:MYSQL_PASSWORD}` or `${file:/run/secrets/db_pass}`. Netdata resolves it at job startup.

:::

## Supported Methods

### Environment Variables

Reference an environment variable using the `${env:VARIABLE_NAME}` syntax:

```yaml
jobs:
  - name: local
    dsn: "${env:MYSQL_USER}:${env:MYSQL_PASSWORD}@tcp(127.0.0.1:3306)/"
```

**Shorthand syntax:** For variables whose names contain only uppercase letters, digits, and underscores (matching `[A-Z_][A-Z0-9_]*`), you can omit the `env:` prefix:

```yaml
jobs:
  - name: prod_db
    dsn: "${PGUSER}:${PGPASSWORD}@tcp(${PGHOST}:5432)/mydb"
```

:::note

The shorthand syntax only works for names that match `[A-Z_][A-Z0-9_]*`. Use the explicit `${env:name}` form for variables with lowercase characters.

:::

### File References

Read a secret from a file using the `${file:/path/to/secret}` syntax. The file content is read and leading/trailing whitespace is trimmed.

```yaml
jobs:
  - name: myapp
    url: http://localhost:8080/metrics
    username: admin
    password: "${file:/run/secrets/myapp_password}"
```

This works with:

- Kubernetes Secrets mounted as files
- Docker secrets (`/run/secrets/`)
- Any vault sidecar that writes secrets to the filesystem

### External Commands (Coming Soon)

:::note

External command references are planned for a future release and are not yet available.

:::

Execute any command and use its stdout as the secret value, using the `${cmd:/usr/bin/command args}` syntax:

```yaml
jobs:
  - name: prod
    password: "${cmd:/usr/bin/vault kv get -field=password secret/netdata/mysql}"
```

This will support any vault CLI — HashiCorp Vault, AWS CLI, Azure CLI, and others.

## How It Works

- References are resolved when the collector job starts (or restarts).
- If a reference cannot be resolved, the job **fails to start** with a clear error message.
- The job retries based on its `autodetection_retry` setting, picking up newly available secrets on each attempt.
- Multiple references in a single value are all resolved independently.
- Only string configuration values are scanned for references.
- Resolution is **single-pass** — a resolved value is not scanned again for further references.

## Deployment Examples

### systemd

Create an environment file with your secrets and reference it from a systemd drop-in override:

```ini
# /etc/netdata/secrets.env
MYSQL_PASSWORD=supersecret
PGUSER=netdata
PGPASSWORD=s3cret
```

Create a drop-in override for the Netdata service:

```bash
sudo systemctl edit netdata
```

Add the following:

```ini
[Service]
EnvironmentFile=/etc/netdata/secrets.env
```

Then restart:

```bash
sudo systemctl restart netdata
```

:::important

The `EnvironmentFile` directive makes the variables available only to processes started by this systemd unit. The secrets are not exposed to other users or processes on the system.

:::

### Docker

Pass secrets as environment variables:

```bash
docker run -e MYSQL_PASSWORD=supersecret netdata/netdata
```

Or use Docker secrets with Compose:

```yaml
# docker-compose.yml
services:
  netdata:
    image: netdata/netdata
    secrets:
      - mysql_password

secrets:
  mysql_password:
    file: ./mysql_password.txt
```

Then reference the mounted secret file in your collector config:

```yaml
password: "${file:/run/secrets/mysql_password}"
```

### Kubernetes

**Using Kubernetes Secrets as environment variables:**

```yaml
env:
  - name: MYSQL_PASSWORD
    valueFrom:
      secretKeyRef:
        name: netdata-secrets
        key: mysql-password
```

Then reference in your collector config:

```yaml
password: "${env:MYSQL_PASSWORD}"
```

**Using External Secrets Operator:**

If you use [External Secrets Operator](https://external-secrets.io/) to sync secrets from an external vault into Kubernetes:

```yaml
apiVersion: external-secrets.io/v1
kind: ExternalSecret
spec:
  secretStoreRef:
    name: vault-backend
    kind: SecretStore
  target:
    name: netdata-secrets
  data:
    - secretKey: mysql-password
      remoteRef:
        key: secret/data/netdata/mysql
        property: password
```

The resulting Kubernetes Secret can then be injected as environment variables or mounted as files — both of which Netdata can resolve.

## Vault Integrations

The following table shows how popular vault solutions can provide secrets to Netdata through environment variables or files:

| Vault | Method | How |
|-------|--------|-----|
| HashiCorp Vault | Env/File | Vault Agent sidecar, Vault CSI Provider, or Vault CLI in init container |
| AWS Secrets Manager | Env/File | External Secrets Operator, or AWS Secrets CSI Driver |
| Azure Key Vault | Env/File | External Secrets Operator, or Azure Key Vault CSI Driver |
| GCP Secret Manager | Env/File | External Secrets Operator, or GCP Secret CSI Driver |
| CyberArk | Env/File | Conjur sidecar, CyberArk Secrets Provider |
| 1Password | Env/File | 1Password Connect, or `op` CLI in init script |
| Doppler | Env | Doppler CLI or Kubernetes Operator |
| Infisical | Env/File | Infisical Agent or Kubernetes Operator |

:::tip

You do not need a specific Netdata plugin for any of these vaults. As long as the vault solution can inject secrets as **environment variables** or **files**, Netdata can consume them.

:::

## Troubleshooting

**Error: "environment variable 'X' is not set"**

The variable is not available in Netdata's process environment. For systemd installations, use `EnvironmentFile=` in a drop-in override (see [systemd deployment](#systemd) above).

To verify which environment variables reached the Netdata process:

```bash
tr '\0' '\n' < /proc/$(pidof netdata)/environ | grep VARIABLE_NAME
```

**Error: "reading secret file: open /path: no such file or directory"**

Verify the file path exists and that the `netdata` user has read access to it:

```bash
sudo -u netdata cat /path/to/secret/file
```

**Job keeps restarting but secrets are not available yet**

This is expected. When a secret reference cannot be resolved, the job fails and retries according to its `autodetection_retry` interval. Once the secret becomes available (for example, after a vault sidecar finishes writing it), the job starts successfully on the next retry.
