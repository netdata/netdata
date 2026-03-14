# Secrets Management

Netdata collectors can resolve secrets at startup instead of storing plain-text credentials in configuration files.

| Reference Type       | Syntax                             | Use Case                                                   |
|:---------------------|:-----------------------------------|:-----------------------------------------------------------|
| Environment variable | `${env:VAR_NAME}`                  | Secrets available as env vars                              |
| File                 | `${file:/path/to/secret}`          | Secrets stored in files on disk                            |
| Command              | `${cmd:/path/to/command args}`     | Secrets retrieved by running a command                     |
| Secretstore          | `${store:<kind>:<name>:<operand>}` | Secrets stored in remote backends (Vault, AWS, Azure, GCP) |

## Environment Variables

Use `${env:VARIABLE_NAME}` to resolve an environment variable.

```yaml
jobs:
  - name: local
    dsn: "${env:MYSQL_USER}:${env:MYSQL_PASSWORD}@tcp(127.0.0.1:3306)/"
```

## Files

Use `${file:/absolute/path}` to read a secret from a file. Leading and trailing whitespace is trimmed.

```yaml
jobs:
  - name: myapp
    password: "${file:/run/secrets/myapp_password}"
```

## Commands

Use `${cmd:/absolute/path/to/command args}` to execute a command and use its stdout as the secret value.

```yaml
jobs:
  - name: prod
    password: "${cmd:/usr/bin/op read op://vault/netdata/mysql/password}"
```

:::warning

Command paths must be absolute. Commands have a 10-second timeout.
Arguments are split on whitespace, not shell-parsed. Quoting, pipes, redirects, variable expansion, and other shell syntax are not interpreted unless you run a shell explicitly.

:::

## Secretstores

For remote secret backends, you first configure a **secretstore** and then reference it from collector configs.

### Supported Providers

| Kind       | Provider            | Operand Format                               | Example Operand                      |
|:-----------|:--------------------|:---------------------------------------------|:-------------------------------------|
| `vault`    | HashiCorp Vault     | `path#key`                                   | `secret/data/netdata/mysql#password` |
| `aws-sm`   | AWS Secrets Manager | `secret-name` or `secret-name#key`           | `netdata/mysql#password`             |
| `azure-kv` | Azure Key Vault     | `vault-name/secret-name`                     | `my-keyvault/mysql-password`         |
| `gcp-sm`   | GCP Secret Manager  | `project/secret` or `project/secret/version` | `my-project/mysql-password`          |

### Reference Format

```text
${store:<kind>:<name>:<operand>}
```

| Part      | Description                                       |
|:----------|:--------------------------------------------------|
| `kind`    | Provider kind from the table above                |
| `name`    | Name you gave the secretstore when you created it |
| `operand` | Provider-specific secret path (see table above)   |

### Usage Examples

```yaml
jobs:
  - name: mysql_prod
    password: "${store:vault:vault_prod:secret/data/netdata/mysql#password}"

  - name: redis_prod
    password: "${store:aws-sm:aws_prod:netdata/redis#password}"

  - name: api_prod
    token: "${store:azure-kv:azure_prod:my-vault/api-token}"

  - name: app_prod
    password: "${store:gcp-sm:gcp_prod:my-project/mysql-password}"
```

### Setup Through Dynamic Configuration

1. Create a secretstore through the Netdata Dynamic Configuration UI.
2. Choose a provider kind and give it a name.
3. Provide the provider-specific configuration.
4. Reference it from collector configs using `${store:<kind>:<name>:<operand>}`.

### File-Defined Secretstores

Netdata can also load secretstores from `go.d/ss` during agent startup.

- User-defined files live under `/etc/netdata/go.d/ss/`.
- Stock examples live under `/usr/lib/netdata/conf.d/go.d/ss/`.
- Use one file per backend with a top-level `jobs:` array.
- `kind` is inferred from the filename and must not be written inside the job payload.

| File          | Secretstore Kind |
|:--------------|:-----------------|
| `vault.conf`     | `vault`          |
| `aws-sm.conf`    | `aws-sm`         |
| `azure-kv.conf`  | `azure-kv`       |
| `gcp-sm.conf`    | `gcp-sm`         |

Example:

```yaml
jobs:
  - name: vault_prod
    mode: token
    mode_token:
      token: your-vault-token
    addr: https://vault.example
```

:::info File-Defined Behavior

- File-defined secretstores are loaded at startup only. If you edit `go.d/ss` files, restart the agent to apply the changes.
- Secretstores do not support `enable` or `disable`.
- Updating a file-defined secretstore through Dynamic Configuration creates a `dyncfg` override. That runtime state can diverge from the file on disk until restart.
- Precedence is strict: `dyncfg` overrides user files, and user files override stock files.
- A higher-priority startup config still wins even if it fails to initialize.
- Removing a `dyncfg` override does not reveal a lower-priority file-defined secretstore until the agent restarts.

:::

:::info Behavior

- Secrets are resolved when a collector job starts or restarts.
- Updating a secretstore automatically restarts any collector jobs that depend on it, so they pick up the new configuration.
- If secret resolution fails, the collector job fails to start with an error.

:::

:::tip Security

- Only string configuration values are scanned for secret references.
- Secret resolution is single-pass — a resolved value is not scanned again.
- Avoid storing plain-text credentials in collector configs when an environment variable, file, command, or secretstore can be used instead.

:::
