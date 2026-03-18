# Secrets Management

Netdata supports secrets management for collector configurations, so you don't need to store plain-text credentials in configuration files. Instead, you use secret references that are resolved when a collector starts.

| Reference Type       | Syntax                             | Use Case                                                   |
|:---------------------|:-----------------------------------|:-----------------------------------------------------------|
| Environment variable | `${env:VAR_NAME}`                  | Secrets available as environment variables                 |
| File                 | `${file:/path/to/secret}`          | Secrets stored in files on disk                            |
| Command              | `${cmd:/path/to/command args}`     | Secrets retrieved by running a command                     |
| Secretstore          | `${store:<kind>:<name>:<operand>}` | Secrets stored in remote backends (Vault, AWS, Azure, GCP) |

## Environment Variables

Use `${env:VARIABLE_NAME}` to reference an environment variable.

```yaml
jobs:
  - name: local
    dsn: "${env:MYSQL_USER}:${env:MYSQL_PASSWORD}@tcp(127.0.0.1:3306)/"
```

## Files

Use `${file:/absolute/path}` to read a secret from a file. Leading and trailing whitespace is trimmed automatically.

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

- Command paths must be absolute.
- Commands have a 10-second timeout.
- Arguments are split on whitespace. Quoting, pipes, redirects, and variable expansion are not interpreted unless you run a shell explicitly (e.g., `${cmd:/bin/sh -c "your command here"}`).

:::

## Secretstores

For remote secret backends (HashiCorp Vault, AWS Secrets Manager, Azure Key Vault, GCP Secret Manager), you configure a **secretstore** and then reference it from collector configurations.

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

| Part      | Description                                              |
|:----------|:---------------------------------------------------------|
| `kind`    | Provider kind from the table above (e.g., `vault`)       |
| `name`    | The name you gave the secretstore when you configured it |
| `operand` | Provider-specific path to the secret (see table above)   |

### Examples

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

### Configuring a Secretstore

#### Option 1: Dynamic Configuration (UI)

1. Open the Netdata Dynamic Configuration UI.
2. Choose a provider kind and give your secretstore a name.
3. Fill in the provider-specific settings (address, credentials, etc.).
4. Use the reference syntax `${store:<kind>:<name>:<operand>}` in your collector configs.

#### Option 2: Configuration Files

You can define secretstores in configuration files. Each provider has its own file:

| File                                 | Provider            |
|:-------------------------------------|:--------------------|
| `/etc/netdata/go.d/ss/vault.conf`    | HashiCorp Vault     |
| `/etc/netdata/go.d/ss/aws-sm.conf`   | AWS Secrets Manager |
| `/etc/netdata/go.d/ss/azure-kv.conf` | Azure Key Vault     |
| `/etc/netdata/go.d/ss/gcp-sm.conf`   | GCP Secret Manager  |

Each file contains a `jobs` array. The provider kind is determined by the filename.

Example (`/etc/netdata/go.d/ss/vault.conf`):

```yaml
jobs:
  - name: vault_prod
    mode: token
    mode_token:
      token: your-vault-token
    addr: https://vault.example.com
```

:::note

File-based secretstores are loaded at agent startup. If you edit these files, restart the Netdata Agent to apply the changes.

:::

## How It Works

- Secrets are resolved each time a collector job starts or restarts.
- If a secret cannot be resolved, the collector job will fail to start and log an error.
- Updating a secretstore automatically restarts running and failed collector jobs that use it, so they pick up the new credentials.
- Accepted or disabled jobs keep their state and use the updated secretstore the next time they start.
- If a secretstore change applies successfully but some dependent collector restarts fail, the command reports those restart failures.

:::tip

Avoid storing plain-text credentials in collector configurations. Use environment variables, files, commands, or secretstores instead.

:::
