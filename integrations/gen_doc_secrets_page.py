"""
Generate src/collectors/SECRETS.md from integrations/integrations.js.

This script:
- reads discovered secretstore integrations from integrations.js;
- renders a shared Secrets Management entry page;
- keeps shared resolver documentation in one dedicated configuration block.
"""

from __future__ import annotations

import json
import pathlib
import re
from typing import Any, Dict, List

GITHUB_BLOB_PREFIX = "https://github.com/netdata/netdata/blob/master"
TEMPLATE_PATH = pathlib.Path(__file__).resolve().parent / "templates"

SECRETS_PAGE = {
    "title": "# Secrets Management",
    "intro": [
        "Keep collector credentials out of plain-text configuration files.",
        "Netdata lets you reference secret values in collector configs instead of storing them directly in YAML. "
        "Depending on where the secret lives, you can resolve it from environment variables, local files, local "
        "commands, or remote secretstore backends.",
    ],
    "jump_to": [
        {"label": "Resolver Quick Reference", "anchor": "resolver-quick-reference"},
        {"label": "Choosing a Resolver", "anchor": "choosing-a-resolver"},
        {"label": "Environment Variables", "anchor": "environment-variables"},
        {"label": "Files", "anchor": "files"},
        {"label": "Commands", "anchor": "commands"},
        {"label": "Secretstores", "anchor": "secretstores"},
        {"label": "Supported Secretstore Backends", "anchor": "supported-secretstore-backends"},
        {"label": "How It Works", "anchor": "how-it-works"},
        {"label": "Troubleshooting", "anchor": "troubleshooting"},
    ],
    "quick_reference": [
        {
            "resolver": "Environment variable",
            "syntax": "`${env:VAR_NAME}`",
            "best_for": "Secrets already injected into the Netdata service environment",
            "notes": "Value is trimmed. The variable must exist.",
        },
        {
            "resolver": "File",
            "syntax": "`${file:/absolute/path}`",
            "best_for": "Secrets stored in local files on disk",
            "notes": "The path must be absolute. File contents are trimmed.",
        },
        {
            "resolver": "Command",
            "syntax": "`${cmd:/absolute/path/to/command args}`",
            "best_for": "Secrets returned by a trusted local command",
            "notes": "The command path must be absolute. Netdata uses a 10-second timeout.",
        },
        {
            "resolver": "Secretstore",
            "syntax": "`${store:<kind>:<name>:<operand>}`",
            "best_for": "Secrets stored in remote backends such as Vault, AWS, Azure, or GCP",
            "notes": "Configure the secretstore first, then reference it from collector configs.",
        },
    ],
    "choosing_a_resolver": [
        "Use `${env:...}` or `${file:...}` for simple setups where secrets are already available locally on the Netdata host.",
        "Use `${cmd:...}` when you need dynamic secret retrieval via a trusted local command, such as 1Password CLI or a custom script.",
        "Use `${store:...}` when your organization manages secrets centrally in a cloud provider or Vault and you want Netdata to pull from that source directly.",
        "You can use different resolver types across different collectors, different jobs within the same collector, or even within the same configuration value. See [Mixing resolver types](#mixing-resolver-types).",
    ],
    "sections": {
        "env": {
            "heading": "## Environment Variables",
            "body": "Use `${env:VARIABLE_NAME}` to read a secret from the Netdata process environment.",
            "example": """```yaml
jobs:
  - name: mysql_prod
    password: "${env:MYSQL_PASSWORD}"
```""",
            "notes": [
                "Netdata trims leading and trailing whitespace from the environment variable value.",
                "The variable must be set in the environment of the Netdata service or process that runs the collector.",
            ],
        },
        "file": {
            "heading": "## Files",
            "body": "Use `${file:/absolute/path}` to read a secret from a local file on disk.",
            "example": """```yaml
jobs:
  - name: mysql_prod
    password: "${file:/run/secrets/mysql_password}"
```""",
            "notes": [
                "The file path must be absolute.",
                "Netdata trims leading and trailing whitespace from the file contents.",
                "The file must exist on the Netdata host and be readable by the `netdata` user.",
                "**Docker Secrets**: Docker mounts secrets as files under `/run/secrets/` inside the container. Use `${file:/run/secrets/<secret-name>}` to read them.",
                "**Kubernetes Secrets**: If you mount Kubernetes Secrets as volume files in the Netdata pod, reference them with `${file:/path/to/mounted/secret}`.",
            ],
        },
        "cmd": {
            "heading": "## Commands",
            "body": "Use `${cmd:/absolute/path/to/command args}` to execute a trusted local command and use its stdout as the secret value.",
            "example": """```yaml
jobs:
  - name: mysql_prod
    password: "${cmd:/usr/bin/op read op://vault/netdata/mysql/password}"
```""",
            "notes": [
                "The command path must be absolute.",
                "Arguments are split on whitespace. Netdata does not interpret shell quoting, pipes, redirects, or variable expansion unless you explicitly run a shell such as `/bin/sh -c`.",
                "Netdata uses a 10-second timeout for command resolvers.",
                "Netdata trims leading and trailing whitespace from stdout and ignores stderr.",
            ],
        },
    },
    "store": {
        "heading": "## Secretstores",
        "body": "Use secretstores when you want Netdata collectors to fetch secrets from remote backends at runtime instead of storing them locally in collector configs.",
        "reference_intro": "Configure a secretstore first, then reference it from collector configs with:",
        "reference_syntax": "${store:<kind>:<name>:<operand>}",
        "reference_parts": [
            {"name": "`kind`", "description": "Secretstore backend kind, such as `vault` or `aws-sm`."},
            {"name": "`name`", "description": "The store name you configured in Netdata, such as `vault_prod`."},
            {"name": "`operand`", "description": "Backend-specific identifier for the secret you want to read."},
        ],
        "example": """```yaml
jobs:
  - name: mysql_prod
    password: "${store:vault:vault_prod:secret/data/netdata/mysql#password}"
```""",
        "ui_steps": [
            "Open the Netdata Dynamic Configuration UI.",
            "Go to `Collectors -> go.d -> SecretStores`.",
            "Choose the backend kind you want to configure.",
            "Give the secretstore a name.",
            "Fill in the backend-specific settings.",
            "Save the secretstore and use its `${store:<kind>:<name>:<operand>}` reference in collector configs.",
        ],
        "file_intro": "Each secretstore backend has its own file under `/etc/netdata/go.d/ss/`:",
        "file_note": "File-based secretstores are loaded at agent startup. If you edit these files, restart the Netdata Agent to apply the changes.",
        "file_directory": (
            "If the `/etc/netdata/go.d/ss/` directory does not exist, create it:\n\n"
            "```bash\n"
            "sudo mkdir -p /etc/netdata/go.d/ss\n"
            "sudo chown netdata:netdata /etc/netdata/go.d/ss\n"
            "sudo chmod 0750 /etc/netdata/go.d/ss\n"
            "```\n\n"
            "Secretstore configuration files may contain sensitive values such as tokens or client secrets. "
            "Restrict directory and file permissions to the `netdata` user."
        ),
        "mixing": (
            "You can mix different resolver types in the same configuration value or the same config file. "
            "For example, you might read the username from an environment variable and the password from a secretstore:\n\n"
            "```yaml\n"
            "jobs:\n"
            "  - name: mysql_prod\n"
            '    dsn: "${env:MYSQL_USER}:${store:vault:vault_prod:secret/data/netdata/mysql#password}@tcp(127.0.0.1:3306)/"\n'
            "```\n\n"
            "Different jobs within the same collector config file can also use different resolver types."
        ),
        "multiple_stores": (
            "Each secretstore config file can contain multiple `jobs` entries, each with a unique store name. "
            "You can use different secretstore backends simultaneously. "
            "For example, you might configure a Vault store for database credentials and an AWS Secrets Manager store for API keys, "
            "then reference each one using its `${store:<kind>:<name>:<operand>}` syntax in the relevant collector configs."
        ),
    },
    "secretstores": {
        "heading": "## Supported Secretstore Backends",
        "intro": "Use the backend README for provider-specific authentication, operand rules, configuration examples, and troubleshooting.",
    },
    "how_it_works": [
        "Secrets are resolved each time a collector job starts or restarts.",
        "If a secret cannot be resolved, the collector job will fail to start and log an error.",
        "Updating a secretstore automatically restarts running and failed collector jobs that use it so they pick up the new credentials.",
        "Accepted or disabled jobs keep their state and use the updated secretstore the next time they start.",
        "If a secretstore change applies successfully but some dependent collector restarts fail, Netdata reports those restart failures.",
    ],
    "security_notes": [
        "Prefer secret references over plain-text credentials in collector configs.",
        "Prefer platform-native identity modes for production when a backend supports them, such as instance roles, managed identities, or metadata-based credentials.",
        "Secretstore configuration values (such as tokens and client secrets) also support `${env:...}`, `${file:...}`, and `${cmd:...}` resolvers. Use them to avoid storing backend credentials in plain text. Note that `${store:...}` references are not supported inside secretstore configurations.",
        "Keep local secret material readable only by the `netdata` user, including token files, service account files, and any files used with `${file:...}`.",
        "Use `${cmd:...}` only with trusted local commands and absolute paths.",
    ],
    "troubleshooting": {
        "intro": [
            "Secret resolution failures appear in agent logs and usually surface as collector jobs failing to start.",
            "Start by checking the resolver syntax you used in the collector config.",
            "For `${env:...}`, make sure the variable exists in the Netdata process environment.",
            "For `${file:...}`, make sure the path is absolute and the file is readable by `netdata`.",
            "For `${cmd:...}`, make sure the command path is absolute and the command completes within 10 seconds.",
            "For `${store:...}`, check the backend README for provider-specific operand rules, authentication requirements, and troubleshooting.",
        ],
        "errors": [
            {"syntax": "`${env:VAR_NAME}`", "message": "environment variable is not set"},
            {"syntax": "`${file:relative/path}`", "message": "file path must be absolute"},
            {"syntax": "`${cmd:echo hello}`", "message": "command path must be absolute"},
            {"syntax": "`${cmd:/path/to/slow-command}`", "message": "command timed out after 10s"},
        ],
    },
}


def _extract_integrations_json(js_text: str) -> str:
    """Extract integrations JSON from integrations.js."""
    after_categories = js_text.split("export const categories = ", 1)[1]
    _, after_integrations = after_categories.split("export const integrations = ", 1)
    return re.split(r"\n\s*export const|\Z", after_integrations, maxsplit=1)[0].strip().rstrip(';').strip()


def load_integrations(js_path: str = "integrations/integrations.js") -> Any:
    """Load integrations catalog from JavaScript."""
    with open(js_path, "r", encoding="utf-8") as f:
        js_data = f.read()
    return json.loads(_extract_integrations_json(js_data))


def iterate_integrations(integrations: Any):
    """Yield integration objects from dict or list."""
    if isinstance(integrations, dict):
        for integ in integrations.values():
            if isinstance(integ, dict):
                yield integ
    elif isinstance(integrations, list):
        for integ in integrations:
            if isinstance(integ, dict):
                yield integ


def collect_secretstore_integrations(integrations: Any) -> List[Dict[str, Any]]:
    """Collect secretstore integration entries."""
    items = []

    for integ in iterate_integrations(integrations):
        if integ.get("integration_type") != "secretstore":
            continue
        meta = integ.get("meta", {})
        if not isinstance(meta, dict):
            continue
        if not isinstance(meta.get("name"), str) or not isinstance(meta.get("kind"), str):
            continue
        items.append(integ)

    items.sort(key=lambda item: item["meta"]["name"].lower())
    return items


def get_repo_path_from_blob_url(url: str) -> str:
    """Convert a GitHub blob URL to a repo-root markdown path when possible."""
    if url.startswith(GITHUB_BLOB_PREFIX):
        return url[len(GITHUB_BLOB_PREFIX):]
    return url


def get_secretstore_readme_link(integ: Dict[str, Any]) -> str:
    """Generate the repo-local README link for a secretstore backend."""
    edit_link = integ.get("edit_link", "") if isinstance(integ, dict) else ""
    repo_path = get_repo_path_from_blob_url(edit_link)
    if repo_path.endswith("/metadata.yaml"):
        return repo_path[: -len("metadata.yaml")] + "README.md"
    return ""


_jinja_env = None


def get_jinja_env():
    """Return the shared Jinja environment used by docs templates."""
    global _jinja_env

    if _jinja_env is None:
        from jinja2 import Environment, FileSystemLoader, select_autoescape

        _jinja_env = Environment(
            loader=FileSystemLoader(TEMPLATE_PATH),
            autoescape=select_autoescape(),
            block_start_string='[%',
            block_end_string='%]',
            variable_start_string='[[',
            variable_end_string=']]',
            comment_start_string='[#',
            comment_end_string='#]',
            trim_blocks=True,
            lstrip_blocks=True,
        )

    return _jinja_env


def build_secretstores_context(integrations: Any) -> List[Dict[str, str]]:
    """Build template context for discovered secretstore backends."""
    items = []

    for integ in collect_secretstore_integrations(integrations):
        meta = integ.get("meta", {})
        summary = integ.get("collector_configs_summary", {})
        if not isinstance(summary, dict):
            summary = {}
        kind = meta.get("kind", "")
        readme_link = get_secretstore_readme_link(integ)
        name = meta.get("name", "")
        items.append(
            {
                "name": name,
                "kind": kind,
                "config_file": f"/etc/netdata/go.d/ss/{kind}.conf",
                "name_link": f'[{name}]({readme_link})',
                "operand_format": summary.get("operand_format", "See backend README"),
                "example_operand": summary.get("example_operand", "See backend README"),
            }
        )

    return items


def build_page_context() -> Dict[str, Any]:
    """Build template-friendly page context."""
    return {
        "title": SECRETS_PAGE["title"],
        "intro": SECRETS_PAGE["intro"],
        "jump_to_line": " • ".join(
            f'[{jump["label"]}](#{jump["anchor"]})' for jump in SECRETS_PAGE["jump_to"]
        ),
        "quick_reference": SECRETS_PAGE["quick_reference"],
        "choosing_a_resolver": SECRETS_PAGE["choosing_a_resolver"],
        "sections": [
            SECRETS_PAGE["sections"]["env"],
            SECRETS_PAGE["sections"]["file"],
            SECRETS_PAGE["sections"]["cmd"],
        ],
        "store": SECRETS_PAGE["store"],
        "secretstores": {
            "heading": SECRETS_PAGE["secretstores"]["heading"],
            "intro": SECRETS_PAGE["secretstores"]["intro"],
        },
        "how_it_works": SECRETS_PAGE["how_it_works"],
        "security_notes": SECRETS_PAGE["security_notes"],
        "troubleshooting": SECRETS_PAGE["troubleshooting"],
    }


def render_secrets_md(integrations: Any) -> str:
    """Render the shared Secrets Management entry page."""
    template = get_jinja_env().get_template("secrets.md")
    return template.render(
        page=build_page_context(),
        secretstores=build_secretstores_context(integrations),
    )


def generate_secrets_md() -> None:
    """Generate SECRETS.md from integrations.js and shared resolver content."""
    integrations = load_integrations()
    content = render_secrets_md(integrations)

    outfile = pathlib.Path("./src/collectors/SECRETS.md")
    outfile.parent.mkdir(parents=True, exist_ok=True)

    tmp = outfile.with_suffix(outfile.suffix + ".tmp")
    tmp.write_text(content.rstrip("\n") + "\n", encoding="utf-8")
    tmp.replace(outfile)


if __name__ == "__main__":
    generate_secrets_md()
