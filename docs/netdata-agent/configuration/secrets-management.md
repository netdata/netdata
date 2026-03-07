# Secrets Management

## Overview

Netdata collectors often need credentials such as database passwords, API tokens, and connection strings. Instead of storing these credentials as plain text in configuration files, you can use **secret references** that are resolved automatically when a collector job starts.

Netdata integrates with **secrets vaults** and credential stores — including HashiCorp Vault, AWS Secrets Manager, Azure Key Vault, GCP Secret Manager, CyberArk Conjur, Keeper Secrets Manager, 1Password, Bitwarden, Doppler, Akeyless, Infisical, Delinea, and any other vault with a CLI — so you never need to put passwords in config files.

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

For security, the command path must be absolute (e.g., `/usr/bin/vault`, not just `vault`). Commands have a 10-second timeout. Arguments are split by whitespace — if an argument contains spaces, use a wrapper script.

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

The `EnvironmentFile` directive makes the variables available to processes started by this systemd unit. This limits scope compared to global shell exports, but privileged users or processes may still inspect process environments via `/proc/PID/environ`.

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

Netdata supports **every vault and secrets manager** — either natively, via CLI command execution, or through environment/file injection. The table below lists popular vault solutions and how to integrate them.

### Vaults with Native API Support

| Vault Provider | Syntax | Details |
|----------------|--------|---------|
| **HashiCorp Vault** (HCP Vault, Vault Enterprise, Vault OSS) | `${vault:path#key}` | [Configuration](#hashicorp-vault) |
| **AWS Secrets Manager** | `${aws-sm:name#key}` | [Configuration](#aws-secrets-manager) |
| **Azure Key Vault** | `${azure-kv:vault-name/secret-name}` | [Configuration](#azure-key-vault) |
| **GCP Secret Manager** (Google Cloud) | `${gcp-sm:project/secret}` | [Configuration](#gcp-secret-manager) |

### Vaults Supported via CLI (`${cmd:...}`)

Any vault with a command-line interface works with Netdata's `${cmd:...}` provider. Here are common examples:

| Vault Provider | Example Command |
|----------------|----------------|
| **HashiCorp Vault** CLI (`vault`) | `${cmd:/usr/bin/vault kv get -field=password secret/myapp}` |
| **AWS CLI** (`aws`) | `${cmd:/usr/bin/aws secretsmanager get-secret-value --secret-id myapp --query SecretString --output text}` |
| **Azure CLI** (`az`) | `${cmd:/usr/bin/az keyvault secret show --name mypass --vault-name myvault --query value -o tsv}` |
| **GCP CLI** (`gcloud`) | `${cmd:/usr/bin/gcloud secrets versions access latest --secret=mypass}` |
| **CyberArk Conjur** CLI (`conjur`) | `${cmd:/usr/bin/conjur variable get -i myapp/password}` |
| **CyberArk Credential Provider** (`clipasswordsdk`) | `${cmd:/opt/CARKaim/sdk/clipasswordsdk GetPassword ...}` |
| **Keeper Secrets Manager** CLI (`ksm`) | `${cmd:/usr/bin/ksm secret notation keeper://record/field}` |
| **1Password** CLI (`op`) | `${cmd:/usr/bin/op read op://vault/item/password}` |
| **Bitwarden** CLI (`bw`) | `${cmd:/usr/bin/bw get password myapp}` |
| **Bitwarden Secrets Manager** CLI (`bws`) | `${cmd:/usr/bin/bws secret get secret-id}` |
| **Doppler** CLI (`doppler`) | `${cmd:/usr/bin/doppler secrets get DB_PASS --plain}` |
| **Delinea/Thycotic DevOps Secrets Vault** CLI (`dsv`) | `${cmd:/usr/bin/dsv secret read --path myapp/db --field password}` |
| **Delinea Secret Server** CLI (`tss`) | `${cmd:/usr/bin/tss secret -id 42 -field password}` |
| **Infisical** CLI (`infisical`) | `${cmd:/usr/bin/infisical secrets get DB_PASS --plain}` |
| **Akeyless** CLI (`akeyless`) | `${cmd:/usr/bin/akeyless get-secret-value -n /myapp/password}` |
| **Fortanix SDKMS** CLI | `${cmd:/usr/bin/sdkms-cli export-secret --name mypass}` |
| **EnvKey** CLI (`envkey`) | `${cmd:/usr/local/bin/envkey-get-secret.sh DB_PASS}` |
| **Passbolt** CLI | `${cmd:/usr/bin/passbolt get secret --id uuid}` |
| Any custom vault script | `${cmd:/usr/local/bin/my-vault-script.sh get password}` |

### Vaults Supported via Sidecars, Operators, or File/Env Injection

These vault solutions inject secrets as environment variables or files, which Netdata reads with `${env:...}` or `${file:...}`:

| Vault Provider | Injection Method |
|----------------|-----------------|
| **HashiCorp Vault Agent** | Sidecar writes secrets to files → `${file:/path}` |
| **Kubernetes External Secrets Operator** (ESO) | Syncs any vault to K8s Secrets → `${env:...}` or `${file:...}` |
| **AWS Systems Manager Parameter Store** (SSM) | Via ESO or ECS task definitions → `${env:...}` |
| **1Password Connect** | K8s operator injects secrets → `${env:...}` or `${file:...}` |
| **Keeper Secrets Manager** | K8s integration or CLI injection → `${env:...}` or `${file:...}` |
| **Doppler** | CLI injects env vars at process startup → `${env:...}` |
| **CyberArk Conjur** | Sidecar writes secrets to shared volume → `${file:/path}` |
| **Sealed Secrets** (Bitnami) | Decrypts to K8s Secrets → `${env:...}` or `${file:...}` |
| **SOPS** (Mozilla) | Decrypts files at deploy time → `${file:...}` |
| **Bank-Vaults** (Banzai Cloud) | Mutating webhook injects env vars → `${env:...}` |
| **Docker Secrets** | Mounted at `/run/secrets/` → `${file:/run/secrets/name}` |
| **Podman Secrets** | Mounted as files → `${file:/path}` |
| **systemd credentials** | `EnvironmentFile=` or `LoadCredential=` → `${env:...}` or `${file:...}` |

:::tip

**Every vault solution is supported.** If your vault has a CLI, use `${cmd:...}`. If it has a sidecar, operator, or agent that writes secrets to files or environment variables, use `${file:...}` or `${env:...}`. For HashiCorp Vault, AWS Secrets Manager, Azure Key Vault, and GCP Secret Manager, use native API integration for the simplest setup.

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

## FAQ

**Q: Does Netdata support my vault?**

Yes. Netdata supports every vault and secrets management solution. If your vault has native support (HashiCorp Vault, AWS Secrets Manager, Azure Key Vault, GCP Secret Manager), use the built-in provider. If your vault has a CLI, use `${cmd:/path/to/cli ...}`. If your vault has a sidecar or operator that injects secrets as environment variables or files, use `${env:...}` or `${file:...}`. See the [Vault Integration Summary](#vault-integration-summary) for details.

**Q: Are secrets stored in plain text anywhere?**

No. Secret references (e.g., `${env:MYSQL_PASSWORD}`) are stored in configuration files and in the Dynamic Configuration Manager. The actual secret values are resolved in memory only when a collector job starts. Netdata never writes resolved secrets to disk, never logs them, and never exposes them through the API or UI. Note that some providers (`${file:...}`, `${env:...}`) read secrets from sources that already exist on the filesystem or in the process environment — those sources are managed by you, not by Netdata.

**Q: Can I use multiple secret references in a single configuration value?**

Yes. Multiple references in a single string are all resolved independently. For example:

```yaml
dsn: "${env:DB_USER}:${env:DB_PASS}@tcp(${env:DB_HOST}:3306)/mydb"
```

**Q: What happens if a secret changes (rotation)?**

Secrets are re-resolved every time a collector job starts or restarts. If you rotate a secret, the new value will be picked up on the next job restart. You can trigger this by restarting the Netdata Agent or by using the `autodetection_retry` setting to have jobs automatically retry at regular intervals.

**Q: Can I use secret references with the Dynamic Configuration Manager?**

Yes. The Dynamic Configuration Manager stores the secret reference (e.g., `${vault:secret/data/myapp#password}`), not the resolved value. When you view or edit a job's configuration through the UI, you see the reference — never the actual secret.

**Q: Do I need to install vault SDKs or libraries?**

No. Native vault providers (HashiCorp Vault, AWS, Azure, GCP) use REST APIs directly — no external SDKs or libraries are needed. For the `${cmd:...}` provider, you only need the vault's CLI tool installed on the system.

**Q: Is the `${cmd:...}` provider secure?**

Yes, with safeguards: commands must use absolute paths (no `PATH` lookup), no shell expansion is performed (preventing command injection), and commands have a 10-second timeout. The command's stdout is captured as the secret value; stderr is discarded.

**Q: Can I use secret references in all collector configuration fields?**

Secret references work in any string configuration value — passwords, DSN strings, API tokens, URLs, headers, usernames, certificates paths, and more. Non-string values (numbers, booleans) are not scanned for references.

**Q: How do I debug secret resolution issues?**

Check the Netdata Agent logs (`/var/log/netdata/collector.log` or `journalctl -u netdata`). When a secret reference fails to resolve, the error message identifies which reference failed and why — without revealing the secret value itself. Common issues include missing environment variables, inaccessible files, and vault authentication failures.

**Q: Does secret resolution add latency to collector startup?**

Environment variable and file resolution are near-instant (<1ms). External commands (`${cmd:...}`) add the command's execution time (typically 100-500ms for vault CLIs). Native vault API calls add network round-trip time. This latency occurs only at job startup, not during ongoing metric collection.
