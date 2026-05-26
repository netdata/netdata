"""
Generate src/collectors/SERVICE-DISCOVERY.md from integrations/integrations.js.

Mirrors gen_doc_secrets_page.py:
- reads discovered service-discovery integrations from integrations.js;
- renders the umbrella Service Discovery page;
- carries the canonical SD model, helper-function reference, and rule
  semantics that per-discoverer pages link back to.
"""

from __future__ import annotations

import json
import pathlib
import re
from typing import Any, Dict, List

GITHUB_BLOB_PREFIX = "https://github.com/netdata/netdata/blob/master"
TEMPLATE_PATH = pathlib.Path(__file__).resolve().parent / "templates"

SD_PAGE = {
    "title": "# Service Discovery",
    "intro": [
        "Stop hand-writing collector jobs for every host, container, or device. Netdata's Service Discovery (SD) finds monitorable targets in your environment and turns them into collector jobs automatically.",
        "Each SD pipeline is a `discoverer:` (where to look) plus a list of `services:` rules (how to turn what was found into collector jobs). The discoverer emits **targets**; the rules render **collector job YAML** from those targets using Go templates.",
    ],
    "jump_to": [
        {"label": "How it works", "anchor": "how-it-works"},
        {"label": "Configuration file structure", "anchor": "configuration-file-structure"},
        {"label": "Rule evaluation semantics", "anchor": "rule-evaluation-semantics"},
        {"label": "Template helper reference", "anchor": "template-helper-reference"},
        {"label": "config_template rendering", "anchor": "config_template-rendering"},
        {"label": "Supported discoverers", "anchor": "supported-discoverers"},
        {"label": "Mixing discoverers", "anchor": "mixing-discoverers"},
        {"label": "Troubleshooting", "anchor": "troubleshooting"},
    ],
    "how_it_works": {
        "heading": "## How it works",
        "intro": [
            "Each discovery pipeline runs in five stages:",
        ],
        "stages": [
            {
                "name": "Discover",
                "description": "The discoverer probes its environment for monitorable things — running containers, listening sockets, kubernetes resources, SNMP devices on a subnet, items returned by an HTTP endpoint. What gets probed and how often is controlled by `discoverer:` options.",
            },
            {
                "name": "Build a target",
                "description": "Each discovered thing becomes a target. The target carries discoverer-specific fields (variables) — for example, net_listeners targets have `.Port` / `.Comm` / `.Cmdline`; docker targets have `.Image` / `.Name` / `.Labels`; snmp targets have `.IPAddress` / `.SysInfo.*` / `.Credential.*`. Per-discoverer pages list the full variable set.",
            },
            {
                "name": "Match against services rules",
                "description": "The rule engine evaluates each `services:` rule against each target, top to bottom. A rule's `match` expression is a Go template that must render to the literal string `\"true\"` for the rule to apply.",
            },
            {
                "name": "Render the collector job",
                "description": "When a rule matches, its `config_template` is executed with the target as context, producing collector job YAML. The rendered YAML can be a single job map or a sequence of jobs.",
            },
            {
                "name": "Hand off to the collector",
                "description": "The agent registers the rendered jobs with the matching collector module. From this point on, the collector runs the job — the discoverer is no longer involved.",
            },
        ],
    },
    "config_file": {
        "heading": "## Configuration file structure",
        "intro": [
            "Each discoverer has its own file under `/etc/netdata/go.d/sd/`. The filename determines the discoverer kind (`net_listeners.conf`, `docker.conf`, `http.conf`, `snmp.conf`, `k8s.conf`).",
            "Every SD file has the same shape:",
        ],
        "skeleton": """```yaml
disabled: no                  # set to yes to disable this pipeline

discoverer:
  <kind>:                     # discoverer kind (must match filename)
    # discoverer-specific options — see the per-discoverer page

services:
  - id: <rule-id>
    match: <go-template>      # must render to the literal string "true"
    config_template: |        # optional — omit to make this a "skip rule"
      # collector job YAML, rendered with target as context
```""",
        "notes": [
            "`disabled: yes` keeps the file on disk but turns the pipeline off.",
            "Editing a stock file requires restarting the agent. UI-managed pipelines apply live.",
            "Where each discoverer's stock conf ships (with the Netdata package, with the Helm chart, or not at all) is documented on its per-discoverer page.",
        ],
    },
    "rule_eval": {
        "heading": "## Rule evaluation semantics",
        "intro": [
            "For each discovered target, the engine walks the `services:` array top-to-bottom. The two rule shapes behave differently:",
        ],
        "shapes": [
            {
                "name": "Skip rule (no `config_template`)",
                "description": "When a skip rule matches, the target is dropped immediately — no jobs are produced and **no further rules run for that target**. Use skip rules to exclude targets the catch-all would otherwise pick up. Place them **before** any template rule.",
            },
            {
                "name": "Template rule (with `config_template`)",
                "description": "When a template rule matches, its `config_template` is rendered into one or more collector jobs and rule evaluation **continues** with the next rule. A single target can therefore produce jobs from multiple matching template rules.",
            },
        ],
        "ordering": [
            "Recommended ordering for a multi-rule pipeline:",
            "",
            "1. Skip rules — drop targets you don't want monitored.",
            "2. Specific template rules — vendor-specific, label-specific, port-specific.",
            "3. A catch-all template rule (`match: '{{ true }}'`) — the default fallback.",
            "",
            "If a specific template rule already produced the right job, follow it with a skip rule keyed on the same condition to suppress the catch-all for those targets.",
        ],
    },
    "helpers": {
        "heading": "## Template helper reference",
        "intro": [
            "Match expressions and config templates are [Go `text/template`](https://pkg.go.dev/text/template) bodies with three additional helper sets: the standard Go template builtins, the [sprig](https://masterminds.github.io/sprig/) function library, and a small set of Netdata-specific helpers.",
            "All templates run with `missingkey=error`. Referencing a field that does not exist on the target type (e.g. typo `.SysInfoo.Name` instead of `.SysInfo.Name`) makes the template execution fail; the agent logs the error and skips that rule for that target.",
        ],
        "go_builtins": {
            "heading": "### Go template builtins",
            "intro": "Standard Go `text/template` syntax: pipelines, conditionals, loops, variable assignment, whitespace control.",
            "table": [
                {"name": "`if`/`else if`/`else`/`end`", "description": "Conditional rendering."},
                {"name": "`range`/`end`", "description": "Iterate over a slice or map."},
                {"name": "`with`/`end`", "description": "Set the dot to a value if it is non-empty."},
                {"name": "`{{- ... -}}`", "description": "Whitespace-trim left/right around the action."},
                {"name": "`{{ $var := ... }}`", "description": "Variable assignment, scoped to the enclosing block."},
                {"name": "`eq`, `ne`, `lt`, `le`, `gt`, `ge`", "description": "Comparison. `eq A B C ...` is true if `A` equals **any** of the following arguments."},
                {"name": "`and`, `or`, `not`", "description": "Boolean composition (variadic)."},
                {"name": "`index`", "description": "Index a slice or map: `index .Labels \"app\"`."},
                {"name": "`printf`", "description": "Formatted string."},
            ],
        },
        "sprig": {
            "heading": "### Sprig functions",
            "intro": "The full [sprig library](https://masterminds.github.io/sprig/) is included. The functions most commonly used in stock SD configs:",
            "table": [
                {"name": "`default DEFAULT VALUE`", "description": "Return `VALUE` if non-empty; otherwise `DEFAULT`."},
                {"name": "`empty VALUE`", "description": "True if the value is the zero value for its type."},
                {"name": "`hasKey MAP KEY`", "description": "True if `MAP` contains `KEY`. Used heavily by the `http` discoverer."},
                {"name": "`kindIs KIND VALUE`", "description": "True if `VALUE`'s reflect kind matches: `string`, `map`, `slice`, `bool`, …"},
                {"name": "`lower S` / `upper S`", "description": "Lowercase / uppercase a string."},
                {"name": "`trim S` / `trimPrefix PREFIX S` / `trimSuffix SUFFIX S`", "description": "Whitespace and prefix/suffix trimming."},
                {"name": "`replace OLD NEW S`", "description": "String replace."},
                {"name": "`regexFind RE S`", "description": "Return the first regex match, or empty."},
                {"name": "`regexMatch RE S`", "description": "True if `S` matches `RE` (substring match unless anchored)."},
                {"name": "`printf FMT V...`", "description": "Same as Go's `fmt.Sprintf`."},
            ],
            "outro": "See the [sprig docs](https://masterminds.github.io/sprig/) for the full list (string, math, encoding, list, dict, date helpers).",
        },
        "netdata": {
            "heading": "### Netdata helpers",
            "intro": "Custom helpers added to the SD template engine:",
            "table": [
                {
                    "name": "`match TYPE VALUE PATTERN [PATTERN...]`",
                    "description": "Returns the string `\"true\"` if `VALUE` matches **any** of the patterns under the named matcher type. `TYPE` is one of:",
                    "sub": [
                        "`\"glob\"` — shell glob (`*`, `?`, `[abc]`).",
                        "`\"sp\"` — Netdata simple patterns (space-separated, `*` wildcard, `!` for negation).",
                        "`\"re\"` — RE2 regular expression. Matches if the regex matches anywhere in `VALUE` unless explicitly anchored with `^` / `$`.",
                        "`\"dstar\"` — [doublestar](https://github.com/bmatcuk/doublestar) glob (supports `**` for path-style matching).",
                    ],
                },
                {
                    "name": "`glob VALUE PATTERN [PATTERN...]`",
                    "description": "Shortcut for `match \"glob\" VALUE PATTERN...`.",
                },
                {
                    "name": "`promPort PORT`",
                    "description": "Returns the well-known Prometheus exporter module name registered for `PORT`, or the empty string. `net_listeners`-specific.",
                },
                {
                    "name": "`toYaml VALUE`",
                    "description": "Serialize `VALUE` as a YAML string. Used by `http` discoverer rules that pass through items as collector job configs.",
                },
            ],
            "notes": [
                "`match` and `glob` are case-sensitive.",
                "`match \"re\" ...` is unanchored. Use `^...$` to require a full-string match.",
                "There is **no** standalone `regexp` or `regex` helper. Use `match \"re\"` (or sprig's `regexFind` / `regexMatch`) for regex matching.",
                "`match` and `glob` return the **string** `\"true\"` or `\"false\"`, not a Go bool. The rule engine compares the trimmed template output against the literal `\"true\"`, so a top-level `match: '{{ glob .X \"foo*\" }}'` works directly.",
                "**Composing with `if` / `and` / `or` is a footgun**: in Go templates a non-empty string is truthy, so both `\"true\"` *and* `\"false\"` evaluate truthy under `if`. `{{ if glob .X \"foo*\" }}` is therefore **wrong**. Either wrap each result with `eq ... \"true\"` to get a real bool, or use an explicit `if-then-true` block:\n  ```\n  match: '{{ if and (eq (glob .Vendor \"Cisco*\") \"true\") (eq .Category \"router\") }}true{{ end }}'\n  ```",
            ],
        },
    },
    "config_template": {
        "heading": "## config_template rendering",
        "intro": [
            "When a template rule matches, its `config_template` is executed with the target as the dot context. The rendered output is parsed as YAML to produce one or more collector jobs.",
        ],
        "rules": [
            {
                "name": "Single map → one job",
                "description": "If the rendered YAML is a map, a single collector job is created.",
            },
            {
                "name": "Sequence of maps → one job per element",
                "description": "If the rendered YAML is a sequence (top-level YAML `-` items), one job is created per element. Use this for multi-job rules. Example: a `net_listeners` rule that emits both a TCP and a Unix-socket MySQL job from the same target —\n\n  ```yaml\n  - id: mysql\n    match: '{{ or (eq .Port \"3306\") (eq .Comm \"mysqld\") }}'\n    config_template: |\n      - name: local\n        dsn: netdata@unix(/var/run/mysqld/mysqld.sock)/\n      - name: local\n        dsn: netdata@tcp({{.Address}})/\n  ```",
            },
            {
                "name": "Module inference from rule `id`",
                "description": "If the rendered job map has no `module:` key, the rule's `id` is used as the module name. Set `id: snmp` (or `docker`, `http`, …) to omit `module:` from your template; otherwise include `module:` explicitly.",
            },
            {
                "name": "`id` uniqueness",
                "description": "Rule IDs are not required to be unique across rules — multiple rules can share an `id`. The `id` is used for module inference and shows up in agent logs to help you identify which rule produced a job. Pick descriptive IDs (`cisco`, `hp-printers`, `skip-vips`).",
            },
            {
                "name": "Failure handling",
                "description": "Render errors, YAML parse errors, and `missingkey=error` failures are logged at warn level. The agent skips the rule for that target and continues evaluation.",
            },
        ],
    },
    "supported": {
        "heading": "## Supported discoverers",
        "intro": "Each discoverer has its own page covering its options, target variables, evaluation specifics, and worked examples.",
    },
    "mixing": {
        "heading": "## Mixing discoverers",
        "body": [
            "All discoverers can run simultaneously. Each `/etc/netdata/go.d/sd/<kind>.conf` is independent. The same target can theoretically be discovered by more than one discoverer (for example, a containerised application appears in both `docker` and `net_listeners`); each discoverer's pipeline is independent and may produce its own job.",
            "Use `disabled: yes` at the top of a stock file to keep it on disk but turn the pipeline off.",
            "UI-managed pipelines and file-based pipelines coexist. UI-managed pipelines apply live; file-based pipelines require an agent restart to reload.",
        ],
    },
    "troubleshooting": {
        "heading": "## Troubleshooting",
        "intro": [
            "Common cross-discoverer problems. For discoverer-specific issues, see the per-discoverer page.",
        ],
        "problems": [
            {
                "name": "No targets discovered",
                "description": "Check the agent log for `discoverer=<kind>` lines. Confirm `disabled: no` and that the discoverer's prerequisites are met (network reachability, credentials, API access).",
            },
            {
                "name": "Targets discovered but no jobs created",
                "description": "Check that at least one `services:` rule has both a matching `match` expression and a `config_template`. A rule with no `config_template` is a skip rule — it drops the target instead of producing a job.",
            },
            {
                "name": "`match` always evaluates false",
                "description": "Match expressions must render to the literal string `\"true\"`. A bare `{{ if ... }}` block that does not output anything renders to the empty string, which is treated as false. Use `{{ true }}` for catch-all, `{{ glob .X \"...\" }}` for pattern matches, or `{{ if ... }}true{{ end }}` for ad-hoc conditions.",
            },
            {
                "name": "Template render error",
                "description": "Look for `failed to execute services[N]->config_template on target` in the log. The most common cause is a typo in a variable reference (e.g. `.SysInfoo.Name`) — `missingkey=error` rejects unknown fields. Use the per-discoverer page's variable table to verify spelling.",
            },
            {
                "name": "YAML parse error after rendering",
                "description": "`failed to parse services[N] template data` means the rendered output is not valid YAML. Common cause: a discovered string field contains a colon, hash, or other YAML special character. YAML-quote dynamic values (`name: \"{{ .X }}\"`) when they may be irregular.",
            },
        ],
    },
}


def _extract_integrations_json(js_text: str) -> str:
    after_categories = js_text.split("export const categories = ", 1)[1]
    _, after_integrations = after_categories.split("export const integrations = ", 1)
    return re.split(r"\n\s*export const|\Z", after_integrations, maxsplit=1)[0].strip().rstrip(';').strip()


def load_integrations(js_path: str = "integrations/integrations.js") -> Any:
    with open(js_path, "r", encoding="utf-8") as f:
        js_data = f.read()
    return json.loads(_extract_integrations_json(js_data))


def iterate_integrations(integrations: Any):
    if isinstance(integrations, dict):
        for integ in integrations.values():
            if isinstance(integ, dict):
                yield integ
    elif isinstance(integrations, list):
        for integ in integrations:
            if isinstance(integ, dict):
                yield integ


def collect_sd_integrations(integrations: Any) -> List[Dict[str, Any]]:
    items = []
    for integ in iterate_integrations(integrations):
        if integ.get("integration_type") != "service_discovery":
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
    if url.startswith(GITHUB_BLOB_PREFIX):
        return url[len(GITHUB_BLOB_PREFIX):]
    return url


def get_sd_readme_link(integ: Dict[str, Any]) -> str:
    edit_link = integ.get("edit_link", "") if isinstance(integ, dict) else ""
    repo_path = get_repo_path_from_blob_url(edit_link)
    if repo_path.endswith("/metadata.yaml"):
        return repo_path[: -len("metadata.yaml")] + "README.md"
    return ""


_jinja_env = None


def get_jinja_env():
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


def build_discoverers_context(integrations: Any) -> List[Dict[str, str]]:
    items = []
    for integ in collect_sd_integrations(integrations):
        meta = integ.get("meta", {})
        kind = meta.get("kind", "")
        name = meta.get("name", "")
        tagline = meta.get("tagline", "")
        readme_link = get_sd_readme_link(integ)
        items.append({
            "name": name,
            "kind": kind,
            "config_file": f"/etc/netdata/go.d/sd/{kind}.conf",
            "name_link": f'[{name}]({readme_link})' if readme_link else name,
            "tagline": tagline,
        })
    return items


def build_page_context() -> Dict[str, Any]:
    return {
        "title": SD_PAGE["title"],
        "intro": SD_PAGE["intro"],
        "jump_to_line": " • ".join(
            f'[{jump["label"]}](#{jump["anchor"]})' for jump in SD_PAGE["jump_to"]
        ),
        "how_it_works": SD_PAGE["how_it_works"],
        "config_file": SD_PAGE["config_file"],
        "rule_eval": SD_PAGE["rule_eval"],
        "helpers": SD_PAGE["helpers"],
        "config_template": SD_PAGE["config_template"],
        "supported": SD_PAGE["supported"],
        "mixing": SD_PAGE["mixing"],
        "troubleshooting": SD_PAGE["troubleshooting"],
    }


def render_sd_md(integrations: Any) -> str:
    template = get_jinja_env().get_template("service_discovery.md")
    return template.render(
        page=build_page_context(),
        discoverers=build_discoverers_context(integrations),
    )


def generate_sd_md() -> None:
    integrations = load_integrations()
    content = render_sd_md(integrations)

    outfile = pathlib.Path("./src/collectors/SERVICE-DISCOVERY.md")
    outfile.parent.mkdir(parents=True, exist_ok=True)

    tmp = outfile.with_suffix(outfile.suffix + ".tmp")
    tmp.write_text(content.rstrip("\n") + "\n", encoding="utf-8")
    tmp.replace(outfile)


if __name__ == "__main__":
    generate_sd_md()
