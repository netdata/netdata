# Secret Reference Grammar And Resolution Contract

Scope: the `${...}` secret-reference grammar used in go.d collector configs, how
references resolve, and the optional value-encoding modifier. Owner package:
`src/go/plugin/agent/secrets/`.

## Reference grammar

A reference is `${<scheme-token>:<rest>}`.

- `<scheme-token>` is everything before the first `:` inside the braces.
- `<rest>` is everything after the first `:` and is passed to the scheme
  resolver verbatim (it may contain further `:`, `/`, `#`, `+`, etc.).
- A `${...}` with no `:` has no scheme and is left **unchanged** (e.g. shell-style
  `${MY_TOKEN}` is not a secret reference).

Schemes (base scheme = scheme-token with any modifier removed):

| Scheme | `<rest>` meaning | Resolver |
|---|---|---|
| `env` | environment variable name | `resolver/env.go` |
| `file` | absolute file path | `resolver/file.go` |
| `cmd` | absolute command + args (split on whitespace) | `resolver/cmd.go` |
| `store` | `<kind>:<name>:<operand>` for a configured secretstore | jobmgr store callback -> `secretstore` |

`env`/`file`/`cmd` values are whitespace-trimmed. An unknown base scheme is an
error (`unknown secret provider '<scheme>'`).

## Output modifier

The scheme-token MAY carry one output modifier joined by `+`:
`${<scheme>+<modifier>:<rest>}`. The modifier post-processes the resolved value;
it never alters `<rest>`. `+` cannot appear in a base scheme name, so the
modifier never collides with operand content.

- `urienc` — percent-encode the resolved value to the RFC 3986 **unreserved**
  safe set (`A-Za-z0-9` and `-` `.` `_` `~`); every other byte becomes `%XX`
  (uppercase hex, byte-wise, so multi-byte UTF-8 is encoded per byte). Makes the
  value safe to embed in any URI component (DSN userinfo, path, query).
- empty modifier (`${env+:VAR}`) is treated as no modifier (raw value).
- An unknown modifier is an error (`unknown modifier '<modifier>'`), reported
  **before** the backend/value is fetched.

Defaults and compatibility:

- Default is **raw**: without a modifier the resolved value is used exactly as
  stored. Encoding is opt-in; changing the default would corrupt existing
  reserved-character secrets.
- `urienc` is for a single URI component (e.g. a password). Applying it to a
  plain field or an already-complete URL leaves stray percent-encoded text.
- Encoding is applied **after** resolution, at the generic resolver layer, for
  every scheme. The `secretstore` package contains no encoding logic; the store
  resolver receives the modifier-stripped `<rest>`.

Rationale for the encoder: Go stdlib `url.QueryEscape` (space -> `+`) and
`url.PathEscape` (leaves `@`, `:`, and sub-delims) are both unsafe for a password
in URI userinfo, so the unreserved-set encoder is applied directly.

## Two grammar-parse sites MUST stay in sync

The reference grammar is parsed in exactly two places. Both MUST agree on scheme
and modifier splitting, or escaped store references stop triggering
restart-on-update:

1. `resolver/resolver.go` `resolveRef` — resolves the value.
2. `jobmgr/secretstore_deps.go` `extractSecretStoreKeysFromString` — records which
   secretstore each job depends on (drives restart when a store updates).

Both split the scheme-token via the shared `secretresolver.SplitSchemeModifier`.
Any new scheme or modifier MUST be handled in both, and any new place that parses
`${...}` MUST reuse the shared split.

## Documentation

End-user docs live in `src/collectors/SECRETS.md`, which is **generated** from
`integrations/gen_doc_secrets_page.py` + `integrations/templates/secrets.md`.
Edit the generator/template and regenerate; never hand-edit `SECRETS.md`.
