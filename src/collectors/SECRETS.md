# Secrets Management

Keep collector credentials out of plain-text configuration files.

Netdata lets you reference secret values in collector configs instead of storing them directly in YAML. Depending on where the secret lives, you can resolve it from environment variables, local files, local commands, or remote secretstore backends.

### Jump To

[Resolver Quick Reference](#resolver-quick-reference) • [Choosing a Resolver](#choosing-a-resolver) • [Environment Variables](#environment-variables) • [Files](#files) • [Commands](#commands) • [Secretstores](#secretstores) • [Supported Secretstore Backends](#supported-secretstore-backends) • [How It Works](#how-it-works) • [Troubleshooting](#troubleshooting)


## Resolver Quick Reference

| Resolver | Syntax | Best for | Notes |
|:---------|:-------|:---------|:------|
| Environment variable | `${env:VAR_NAME}` | Secrets already injected into the Netdata service environment | Value is trimmed. The variable must exist. |
| File | `${file:/absolute/path}` | Secrets stored in local files on disk | The path must be absolute. File contents are trimmed. |
| Command | `${cmd:/absolute/path/to/command args}` | Secrets returned by a trusted local command | The command path must be absolute. Netdata uses a 10-second timeout. |
| Secretstore | `${store:<kind>:<name>:<operand>}` | Secrets stored in remote backends such as Vault, AWS, Azure, or GCP | Configure the secretstore first, then reference it from collector configs. |

## Choosing a Resolver

- Use `${env:...}` or `${file:...}` for simple setups where secrets are already available locally on the Netdata host.
- Use `${cmd:...}` when you need dynamic secret retrieval via a trusted local command, such as 1Password CLI or a custom script.
- Use `${store:...}` when your organization manages secrets centrally in a cloud provider or Vault and you want Netdata to pull from that source directly.
- You can use different resolver types across different collectors, different jobs within the same collector, or even within the same configuration value. See [Mixing resolver types](#mixing-resolver-types).

## Environment Variables

Use `${env:VARIABLE_NAME}` to read a secret from the Netdata process environment.

```yaml
jobs:
  - name: mysql_prod
    password: "${env:MYSQL_PASSWORD}"
```

- Netdata trims leading and trailing whitespace from the environment variable value.
- The variable must be set in the environment of the Netdata service or process that runs the collector.

## Files

Use `${file:/absolute/path}` to read a secret from a local file on disk.

```yaml
jobs:
  - name: mysql_prod
    password: "${file:/run/secrets/mysql_password}"
```

- The file path must be absolute.
- Netdata trims leading and trailing whitespace from the file contents.
- The file must exist on the Netdata host and be readable by the `netdata` user.
- **Docker Secrets**: Docker mounts secrets as files under `/run/secrets/` inside the container. Use `${file:/run/secrets/<secret-name>}` to read them.
- **Kubernetes Secrets**: If you mount Kubernetes Secrets as volume files in the Netdata pod, reference them with `${file:/path/to/mounted/secret}`.

## Commands

Use `${cmd:/absolute/path/to/command args}` to execute a trusted local command and use its stdout as the secret value.

```yaml
jobs:
  - name: mysql_prod
    password: "${cmd:/usr/bin/op read op://vault/netdata/mysql/password}"
```

- The command path must be absolute.
- Arguments are split on whitespace. Netdata does not interpret shell quoting, pipes, redirects, or variable expansion unless you explicitly run a shell such as `/bin/sh -c`.
- Netdata uses a 10-second timeout for command resolvers.
- Netdata trims leading and trailing whitespace from stdout and ignores stderr.

## Secretstores

Use secretstores when you want Netdata collectors to fetch secrets from remote backends at runtime instead of storing them locally in collector configs.

Configure a secretstore first, then reference it from collector configs with:

```text
${store:<kind>:<name>:<operand>}
```

| Part | Description |
|:-----|:------------|
| `kind` | Secretstore backend kind, such as `vault` or `aws-sm`. |
| `name` | The store name you configured in Netdata, such as `vault_prod`. |
| `operand` | Backend-specific identifier for the secret you want to read. |

Example:

```yaml
jobs:
  - name: mysql_prod
    password: "${store:vault:vault_prod:secret/data/netdata/mysql#password}"
```

### Configuration Methods

#### Dynamic Configuration UI

1. Open the Netdata Dynamic Configuration UI.
2. Go to `Collectors -> go.d -> SecretStores`.
3. Choose the backend kind you want to configure.
4. Give the secretstore a name.
5. Fill in the backend-specific settings.
6. Save the secretstore and use its `${store:<kind>:<name>:<operand>}` reference in collector configs.

#### Configuration Files

Each secretstore backend has its own file under `/etc/netdata/go.d/ss/`:

| File | Backend |
|:-----|:--------|
| `/etc/netdata/go.d/ss/aws-sm.conf` | AWS Secrets Manager |
| `/etc/netdata/go.d/ss/azure-kv.conf` | Azure Key Vault |
| `/etc/netdata/go.d/ss/gcp-sm.conf` | Google Secret Manager |
| `/etc/netdata/go.d/ss/vault.conf` | Vault |

Each file contains a `jobs` array. The backend kind is determined by the filename.

:::note

File-based secretstores are loaded at agent startup. If you edit these files, restart the Netdata Agent to apply the changes.

:::

If the `/etc/netdata/go.d/ss/` directory does not exist, create it:

```bash
sudo mkdir -p /etc/netdata/go.d/ss
sudo chown netdata:netdata /etc/netdata/go.d/ss
sudo chmod 0750 /etc/netdata/go.d/ss
```

Secretstore configuration files may contain sensitive values such as tokens or client secrets. Restrict directory and file permissions to the `netdata` user.

### Multiple Secretstores

Each secretstore config file can contain multiple `jobs` entries, each with a unique store name. You can use different secretstore backends simultaneously. For example, you might configure a Vault store for database credentials and an AWS Secrets Manager store for API keys, then reference each one using its `${store:<kind>:<name>:<operand>}` syntax in the relevant collector configs.

### Mixing Resolver Types

You can mix different resolver types in the same configuration value or the same config file. For example, you might read the username from an environment variable and the password from a secretstore:

```yaml
jobs:
  - name: mysql_prod
    dsn: "${env:MYSQL_USER}:${store:vault:vault_prod:secret/data/netdata/mysql#password}@tcp(127.0.0.1:3306)/"
```

Different jobs within the same collector config file can also use different resolver types.

## Supported Secretstore Backends

Use the backend README for provider-specific authentication, operand rules, configuration examples, and troubleshooting.

| Backend | Kind | Operand Format | Example Operand |
|:--------|:-----|:---------------|:----------------|
| [AWS Secrets Manager](/src/go/plugin/agent/secrets/secretstore/backends/aws/README.md) | `aws-sm` | `secret-name[#key]` | `netdata/mysql#password` |
| [Azure Key Vault](/src/go/plugin/agent/secrets/secretstore/backends/azure/README.md) | `azure-kv` | `vault-name/secret-name` | `my-keyvault/mysql-password` |
| [Google Secret Manager](/src/go/plugin/agent/secrets/secretstore/backends/gcp/README.md) | `gcp-sm` | `project/secret[/version]` | `my-project/mysql-password` |
| [Vault](/src/go/plugin/agent/secrets/secretstore/backends/vault/README.md) | `vault` | `path#key` | `secret/data/netdata/mysql#password` |

## How It Works

- Secrets are resolved each time a collector job starts or restarts.
- If a secret cannot be resolved, the collector job will fail to start and log an error.
- Updating a secretstore automatically restarts running and failed collector jobs that use it so they pick up the new credentials.
- Accepted or disabled jobs keep their state and use the updated secretstore the next time they start.
- If a secretstore change applies successfully but some dependent collector restarts fail, Netdata reports those restart failures.

## Security Notes

- Prefer secret references over plain-text credentials in collector configs.
- Prefer platform-native identity modes for production when a backend supports them, such as instance roles, managed identities, or metadata-based credentials.
- Secretstore configuration values (such as tokens and client secrets) also support `${env:...}`, `${file:...}`, and `${cmd:...}` resolvers. Use them to avoid storing backend credentials in plain text. Note that `${store:...}` references are not supported inside secretstore configurations.
- Keep local secret material readable only by the `netdata` user, including token files, service account files, and any files used with `${file:...}`.
- Use `${cmd:...}` only with trusted local commands and absolute paths.

## Troubleshooting

- Secret resolution failures appear in agent logs and usually surface as collector jobs failing to start.
- Start by checking the resolver syntax you used in the collector config.
- For `${env:...}`, make sure the variable exists in the Netdata process environment.
- For `${file:...}`, make sure the path is absolute and the file is readable by `netdata`.
- For `${cmd:...}`, make sure the command path is absolute and the command completes within 10 seconds.
- For `${store:...}`, check the backend README for provider-specific operand rules, authentication requirements, and troubleshooting.

Representative error patterns:

- `${env:VAR_NAME}`: environment variable is not set
- `${file:relative/path}`: file path must be absolute
- `${cmd:echo hello}`: command path must be absolute
- `${cmd:/path/to/slow-command}`: command timed out after 10s
