# Secrets Management

## Overview

Netdata collectors often need credentials such as database passwords, API tokens, and connection strings. Instead of storing these credentials as plain text in configuration files, you can use **secret references** that are resolved automatically when a collector job starts.

Secret references work with both YAML configuration files and the [Dynamic Configuration Manager](/docs/netdata-agent/configuration/dynamic-configuration.md). Collectors require zero changes — resolution is fully transparent.

:::tip

**TL;DR** — Replace any plain-text secret in a collector config with a reference like `${env:MYSQL_PASSWORD}`, `${file:/run/secrets/db_pass}`, or `${vault:secret/data/myapp#password}`. Netdata resolves it at job startup.

:::

## Supported Methods

| Method | Syntax | Use Case |
|--------|--------|----------|
| Environment variable | `${env:VAR_NAME}` or `${VAR_NAME}` | systemd, Docker, K8s env injection |
| File | `${file:/path/to/secret}` | K8s mounted secrets, Docker secrets, vault sidecars |
| External command | `${cmd:/usr/bin/command args}` | Any vault CLI |
| HashiCorp Vault | `${vault:path#key}` | Native Vault API integration |
| AWS Secrets Manager | `${aws-sm:secret-name#key}` | Native AWS API integration |
| Azure Key Vault | `${azure-kv:vault-name/secret-name}` | Native Azure API integration |
| GCP Secret Manager | `${gcp-sm:project/secret}` | Native GCP API integration |

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

### External Commands

Execute any command and use its stdout as the secret value using the `${cmd:/path/to/command args}` syntax:

```yaml
jobs:
  - name: prod
    password: "${cmd:/usr/bin/vault kv get -field=password secret/netdata/mysql}"
```

This works with any vault that has a CLI:

```yaml
# HashiCorp Vault
password: "${cmd:/usr/bin/vault kv get -field=password secret/netdata/mysql}"

# AWS CLI
password: "${cmd:/usr/bin/aws secretsmanager get-secret-value --secret-id netdata/mysql --query SecretString --output text}"

# Azure CLI
password: "${cmd:/usr/bin/az keyvault secret show --name mysql-pass --vault-name myvault --query value -o tsv}"

# GCP CLI
password: "${cmd:/usr/bin/gcloud secrets versions access latest --secret=mysql-pass}"

# 1Password CLI
password: "${cmd:/usr/bin/op read op://vault/netdata-mysql/password}"
```

:::important

For security, the command path must be absolute (e.g., `/usr/bin/vault`, not just `vault`). Commands have a 10-second timeout.

:::

### HashiCorp Vault

Access secrets directly from HashiCorp Vault using the `${vault:path#key}` syntax:

```yaml
jobs:
  - name: prod
    password: "${vault:secret/data/netdata/mysql#password}"
```

The part before `#` is the Vault API path, and the part after `#` is the key within the secret data. Both KV v1 and KV v2 secret engines are supported.

**Required environment variables:**

| Variable | Description |
|----------|-------------|
| `VAULT_ADDR` | Vault server address (e.g., `https://vault.example.com`) |
| `VAULT_TOKEN` | Authentication token |

**Optional environment variables:**

| Variable | Description |
|----------|-------------|
| `VAULT_TOKEN_FILE` | Path to file containing the token (fallback: `~/.vault-token`) |
| `VAULT_NAMESPACE` | Vault namespace (for Vault Enterprise) |
| `VAULT_SKIP_VERIFY` | Set to `true` or `1` to skip TLS certificate verification |

### AWS Secrets Manager

Access secrets directly from AWS Secrets Manager using the `${aws-sm:secret-name}` syntax:

```yaml
jobs:
  - name: prod
    # Return the full SecretString
    dsn: "${aws-sm:netdata/mysql-dsn}"

    # Parse SecretString as JSON and extract a specific key
    password: "${aws-sm:netdata/mysql#password}"
```

**Authentication** (tried in order):

1. Environment variables: `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, optional `AWS_SESSION_TOKEN`
2. ECS container credentials: automatic when `AWS_CONTAINER_CREDENTIALS_RELATIVE_URI` is set
3. EC2 Instance Metadata Service (IMDS v2): automatic on EC2 instances with an IAM role

**Required environment variables:**

| Variable | Description |
|----------|-------------|
| `AWS_DEFAULT_REGION` or `AWS_REGION` | AWS region (e.g., `us-east-1`) |

### Azure Key Vault

Access secrets directly from Azure Key Vault using the `${azure-kv:vault-name/secret-name}` syntax:

```yaml
jobs:
  - name: prod
    password: "${azure-kv:my-keyvault/mysql-password}"
```

**Authentication** (tried in order):

1. Client credentials: `AZURE_TENANT_ID`, `AZURE_CLIENT_ID`, `AZURE_CLIENT_SECRET` (all three required)
2. Managed Identity: automatic on Azure VMs/containers. If `AZURE_CLIENT_ID` is set (without the other two), it selects a specific user-assigned managed identity.

### GCP Secret Manager

Access secrets directly from GCP Secret Manager using the `${gcp-sm:project/secret}` syntax:

```yaml
jobs:
  - name: prod
    password: "${gcp-sm:my-project/mysql-password}"

    # Access a specific version
    password: "${gcp-sm:my-project/mysql-password/2}"
```

If no version is specified, `latest` is used.

**Authentication** (tried in order):

1. Metadata server: automatic on GCE/GKE instances with Workload Identity
2. Service account JSON: set `GOOGLE_APPLICATION_CREDENTIALS` to the path of a service account JSON key file

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
VAULT_ADDR=https://vault.example.com
VAULT_TOKEN=hvs.CAESIJlU...
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

## Vault Integration Summary

| Vault | Native Support | Via `${cmd:...}` | Via Env/File |
|-------|---------------|-------------------|--------------|
| HashiCorp Vault | `${vault:path#key}` | `${cmd:/usr/bin/vault ...}` | Vault Agent sidecar |
| AWS Secrets Manager | `${aws-sm:name#key}` | `${cmd:/usr/bin/aws ...}` | External Secrets Operator |
| Azure Key Vault | `${azure-kv:vault/name}` | `${cmd:/usr/bin/az ...}` | External Secrets Operator |
| GCP Secret Manager | `${gcp-sm:project/name}` | `${cmd:/usr/bin/gcloud ...}` | External Secrets Operator |
| CyberArk | — | `${cmd:/usr/bin/conjur ...}` | Conjur sidecar |
| 1Password | — | `${cmd:/usr/bin/op read ...}` | 1Password Connect |
| Doppler | — | `${cmd:/usr/bin/doppler ...}` | Doppler CLI injection |
| Infisical | — | `${cmd:/usr/bin/infisical ...}` | Infisical Agent |

:::tip

Every vault solution is supported. Use native integration for the simplest setup, `${cmd:...}` for any vault with a CLI, or environment variable/file injection via sidecars and operators.

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

**Error: "command path must be absolute"**

The `${cmd:...}` provider requires an absolute path to the command. Use the full path (e.g., `/usr/bin/vault` not just `vault`).

**Error: "VAULT_ADDR environment variable is not set"**

The `${vault:...}` provider requires `VAULT_ADDR` and `VAULT_TOKEN` (or a token file) to be set. See [HashiCorp Vault](#hashicorp-vault) for details.

**Error: "AWS region not set"**

The `${aws-sm:...}` provider requires `AWS_DEFAULT_REGION` or `AWS_REGION`. On EC2/ECS, credentials are obtained automatically from the instance metadata service.

**Job keeps restarting but secrets are not available yet**

This is expected. When a secret reference cannot be resolved, the job fails and retries according to its `autodetection_retry` interval. Once the secret becomes available (for example, after a vault sidecar finishes writing it), the job starts successfully on the next retry.
