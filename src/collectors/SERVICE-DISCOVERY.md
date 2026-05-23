# Service Discovery

Stop hand-writing collector jobs for every host, container, or device. Netdata's Service Discovery (SD) finds monitorable targets in your environment and turns them into collector jobs automatically.

Each SD pipeline is a `discoverer:` (where to look) plus a list of `services:` rules (how to turn what was found into collector jobs). The discoverer emits **targets**; the rules render **collector job YAML** from those targets using Go templates.

### Jump To

[How it works](#how-it-works) • [Configuration file structure](#configuration-file-structure) • [Rule evaluation semantics](#rule-evaluation-semantics) • [Template helper reference](#template-helper-reference) • [config_template rendering](#config_template-rendering) • [Supported discoverers](#supported-discoverers) • [Mixing discoverers](#mixing-discoverers) • [Troubleshooting](#troubleshooting)


## How it works

Each discovery pipeline runs in five stages:

1. **Discover** — The discoverer probes its environment for monitorable things — running containers, listening sockets, kubernetes resources, SNMP devices on a subnet, items returned by an HTTP endpoint. What gets probed and how often is controlled by `discoverer:` options.
2. **Build a target** — Each discovered thing becomes a target. The target carries discoverer-specific fields (variables) — for example, net_listeners targets have `.Port` / `.Comm` / `.Cmdline`; docker targets have `.Image` / `.Name` / `.Labels`; snmp targets have `.IPAddress` / `.SysInfo.*` / `.Credential.*`. Per-discoverer pages list the full variable set.
3. **Match against services rules** — The rule engine evaluates each `services:` rule against each target, top to bottom. A rule's `match` expression is a Go template that must render to the literal string `"true"` for the rule to apply.
4. **Render the collector job** — When a rule matches, its `config_template` is executed with the target as context, producing collector job YAML. The rendered YAML can be a single job map or a sequence of jobs.
5. **Hand off to the collector** — The agent registers the rendered jobs with the matching collector module. From this point on, the collector runs the job — the discoverer is no longer involved.


## Configuration file structure

Each discoverer has its own file under `/etc/netdata/go.d/sd/`. The filename determines the discoverer kind (`net_listeners.conf`, `docker.conf`, `http.conf`, `snmp.conf`, `k8s.conf`).

Every SD file has the same shape:

```yaml
disabled: no                  # set to yes to disable this pipeline

discoverer:
  <kind>:                     # discoverer kind (must match filename)
    # discoverer-specific options — see the per-discoverer page

services:
  - id: <rule-id>
    match: <go-template>      # must render to the literal string "true"
    config_template: |        # optional — omit to make this a "skip rule"
      # collector job YAML, rendered with target as context
```

- `disabled: yes` keeps the file on disk but turns the pipeline off.
- Editing a stock file requires restarting the agent. UI-managed pipelines apply live.
- Where each discoverer's stock conf ships (with the Netdata package, with the Helm chart, or not at all) is documented on its per-discoverer page.


## Rule evaluation semantics

For each discovered target, the engine walks the `services:` array top-to-bottom. The two rule shapes behave differently:

- **Skip rule (no `config_template`)** — When a skip rule matches, the target is dropped immediately — no jobs are produced and **no further rules run for that target**. Use skip rules to exclude targets the catch-all would otherwise pick up. Place them **before** any template rule.
- **Template rule (with `config_template`)** — When a template rule matches, its `config_template` is rendered into one or more collector jobs and rule evaluation **continues** with the next rule. A single target can therefore produce jobs from multiple matching template rules.

### Order matters

Recommended ordering for a multi-rule pipeline:

1. Skip rules — drop targets you don't want monitored.
2. Specific template rules — vendor-specific, label-specific, port-specific.
3. A catch-all template rule (`match: '{{ true }}'`) — the default fallback.

If a specific template rule already produced the right job, follow it with a skip rule keyed on the same condition to suppress the catch-all for those targets.


## Template helper reference

Match expressions and config templates are [Go `text/template`](https://pkg.go.dev/text/template) bodies with three additional helper sets: the standard Go template builtins, the [sprig](https://masterminds.github.io/sprig/) function library, and a small set of Netdata-specific helpers.

All templates run with `missingkey=error`. Referencing a field that does not exist on the target type (e.g. typo `.SysInfoo.Name` instead of `.SysInfo.Name`) makes the template execution fail; the agent logs the error and skips that rule for that target.


### Go template builtins

Standard Go `text/template` syntax: pipelines, conditionals, loops, variable assignment, whitespace control.

| Construct | Description |
|:----------|:------------|
| `if`/`else if`/`else`/`end` | Conditional rendering. |
| `range`/`end` | Iterate over a slice or map. |
| `with`/`end` | Set the dot to a value if it is non-empty. |
| `{{- ... -}}` | Whitespace-trim left/right around the action. |
| `{{ $var := ... }}` | Variable assignment, scoped to the enclosing block. |
| `eq`, `ne`, `lt`, `le`, `gt`, `ge` | Comparison. `eq A B C ...` is true if `A` equals **any** of the following arguments. |
| `and`, `or`, `not` | Boolean composition (variadic). |
| `index` | Index a slice or map: `index .Labels "app"`. |
| `printf` | Formatted string. |


### Sprig functions

The full [sprig library](https://masterminds.github.io/sprig/) is included. The functions most commonly used in stock SD configs:

| Function | Description |
|:---------|:------------|
| `default DEFAULT VALUE` | Return `VALUE` if non-empty; otherwise `DEFAULT`. |
| `empty VALUE` | True if the value is the zero value for its type. |
| `hasKey MAP KEY` | True if `MAP` contains `KEY`. Used heavily by the `http` discoverer. |
| `kindIs KIND VALUE` | True if `VALUE`'s reflect kind matches: `string`, `map`, `slice`, `bool`, … |
| `lower S` / `upper S` | Lowercase / uppercase a string. |
| `trim S` / `trimPrefix PREFIX S` / `trimSuffix SUFFIX S` | Whitespace and prefix/suffix trimming. |
| `replace OLD NEW S` | String replace. |
| `regexFind RE S` | Return the first regex match, or empty. |
| `regexMatch RE S` | True if `S` matches `RE` (substring match unless anchored). |
| `printf FMT V...` | Same as Go's `fmt.Sprintf`. |

See the [sprig docs](https://masterminds.github.io/sprig/) for the full list (string, math, encoding, list, dict, date helpers).


### Netdata helpers

Custom helpers added to the SD template engine:

- `match TYPE VALUE PATTERN [PATTERN...]` — Returns the string `"true"` if `VALUE` matches **any** of the patterns under the named matcher type. `TYPE` is one of:
  - `"glob"` — shell glob (`*`, `?`, `[abc]`).
  - `"sp"` — Netdata simple patterns (space-separated, `*` wildcard, `!` for negation).
  - `"re"` — RE2 regular expression. Matches if the regex matches anywhere in `VALUE` unless explicitly anchored with `^` / `$`.
  - `"dstar"` — [doublestar](https://github.com/bmatcuk/doublestar) glob (supports `**` for path-style matching).
- `glob VALUE PATTERN [PATTERN...]` — Shortcut for `match "glob" VALUE PATTERN...`.
- `promPort PORT` — Returns the well-known Prometheus exporter module name registered for `PORT`, or the empty string. `net_listeners`-specific.
- `toYaml VALUE` — Serialize `VALUE` as a YAML string. Used by `http` discoverer rules that pass through items as collector job configs.

**Notes:**

- `match` and `glob` are case-sensitive.
- `match "re" ...` is unanchored. Use `^...$` to require a full-string match.
- There is **no** standalone `regexp` or `regex` helper. Use `match "re"` (or sprig's `regexFind` / `regexMatch`) for regex matching.
- `match` and `glob` return the **string** `"true"` or `"false"`, not a Go bool. The rule engine compares the trimmed template output against the literal `"true"`, so a top-level `match: '{{ glob .X "foo*" }}'` works directly.
- **Composing with `if` / `and` / `or` is a footgun**: in Go templates a non-empty string is truthy, so both `"true"` *and* `"false"` evaluate truthy under `if`. `{{ if glob .X "foo*" }}` is therefore **wrong**. Either wrap each result with `eq ... "true"` to get a real bool, or use an explicit `if-then-true` block:
  ```
  match: '{{ if and (eq (glob .Vendor "Cisco*") "true") (eq .Category "router") }}true{{ end }}'
  ```


## config_template rendering

When a template rule matches, its `config_template` is executed with the target as the dot context. The rendered output is parsed as YAML to produce one or more collector jobs.

- **Single map → one job** — If the rendered YAML is a map, a single collector job is created.
- **Sequence of maps → one job per element** — If the rendered YAML is a sequence (top-level YAML `-` items), one job is created per element. Use this for multi-job rules. Example: a `net_listeners` rule that emits both a TCP and a Unix-socket MySQL job from the same target —

  ```yaml
  - id: mysql
    match: '{{ or (eq .Port "3306") (eq .Comm "mysqld") }}'
    config_template: |
      - name: local
        dsn: netdata@unix(/var/run/mysqld/mysqld.sock)/
      - name: local
        dsn: netdata@tcp({{.Address}})/
  ```
- **Module inference from rule `id`** — If the rendered job map has no `module:` key, the rule's `id` is used as the module name. Set `id: snmp` (or `docker`, `http`, …) to omit `module:` from your template; otherwise include `module:` explicitly.
- **`id` uniqueness** — Rule IDs are not required to be unique across rules — multiple rules can share an `id`. The `id` is used for module inference and shows up in agent logs to help you identify which rule produced a job. Pick descriptive IDs (`cisco`, `hp-printers`, `skip-vips`).
- **Failure handling** — Render errors, YAML parse errors, and `missingkey=error` failures are logged at warn level. The agent skips the rule for that target and continues evaluation.


## Supported discoverers

Each discoverer has its own page covering its options, target variables, evaluation specifics, and worked examples.

| Discoverer | Kind | Stock conf | Discovers |
|:-----------|:-----|:-----------|:----------|
| [Docker](/src/go/plugin/go.d/discovery/sdext/discoverer/dockersd/README.md) | `docker` | `/etc/netdata/go.d/sd/docker.conf` | Running containers on the local Docker daemon. |
| [HTTP endpoint](/src/go/plugin/go.d/discovery/sdext/discoverer/httpsd/README.md) | `http` | `/etc/netdata/go.d/sd/http.conf` | Items returned by an HTTP/HTTPS endpoint (JSON or YAML). |
| [Kubernetes](/src/go/plugin/go.d/discovery/sdext/discoverer/k8ssd/README.md) | `k8s` | `/etc/netdata/go.d/sd/k8s.conf` | Pods and services in a Kubernetes cluster. |
| [Local listening processes](/src/go/plugin/go.d/discovery/sdext/discoverer/netlistensd/README.md) | `net_listeners` | `/etc/netdata/go.d/sd/net_listeners.conf` | Local processes that listen on TCP/UDP ports. |
| [SNMP](/src/go/plugin/go.d/discovery/sdext/discoverer/snmpsd/README.md) | `snmp` | `/etc/netdata/go.d/sd/snmp.conf` | SNMP-capable devices on configured network subnets. |


## Mixing discoverers

All discoverers can run simultaneously. Each `/etc/netdata/go.d/sd/<kind>.conf` is independent. The same target can theoretically be discovered by more than one discoverer (for example, a containerised application appears in both `docker` and `net_listeners`); each discoverer's pipeline is independent and may produce its own job.

Use `disabled: yes` at the top of a stock file to keep it on disk but turn the pipeline off.

UI-managed pipelines and file-based pipelines coexist. UI-managed pipelines apply live; file-based pipelines require an agent restart to reload.


## Troubleshooting

Common cross-discoverer problems. For discoverer-specific issues, see the per-discoverer page.

### No targets discovered

Check the agent log for `discoverer=<kind>` lines. Confirm `disabled: no` and that the discoverer's prerequisites are met (network reachability, credentials, API access).

### Targets discovered but no jobs created

Check that at least one `services:` rule has both a matching `match` expression and a `config_template`. A rule with no `config_template` is a skip rule — it drops the target instead of producing a job.

### `match` always evaluates false

Match expressions must render to the literal string `"true"`. A bare `{{ if ... }}` block that does not output anything renders to the empty string, which is treated as false. Use `{{ true }}` for catch-all, `{{ glob .X "..." }}` for pattern matches, or `{{ if ... }}true{{ end }}` for ad-hoc conditions.

### Template render error

Look for `failed to execute services[N]->config_template on target` in the log. The most common cause is a typo in a variable reference (e.g. `.SysInfoo.Name`) — `missingkey=error` rejects unknown fields. Use the per-discoverer page's variable table to verify spelling.

### YAML parse error after rendering

`failed to parse services[N] template data` means the rendered output is not valid YAML. Common cause: a discovered string field contains a colon, hash, or other YAML special character. YAML-quote dynamic values (`name: "{{ .X }}"`) when they may be irregular.
