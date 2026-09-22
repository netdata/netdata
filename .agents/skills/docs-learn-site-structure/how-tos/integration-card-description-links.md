# Do links in `metadata.yaml` descriptions follow the same rule as links in docs pages?

Yes. Description and troubleshooting text in a `metadata.yaml` is rendered by `integrations/gen_docs_integrations.py`
into a per-integration `.md` carrying `INTEGRATION_MARKER`; `populate_integrations` splices that page into the map, and
from then on it is an ordinary published page (`../mapping.md#rows-ingest-reads-from-the-map`). Its links go through
`local_to_absolute_links` and `convert_github_links` like every other page (`../mapping.md#links-between-pages`).

Consequences:

- A cross-reference written as a repository-relative `.md` path (for example `/docs/npm/network-flows/configuration.md`)
  is rewritten to the current Learn URL on every ingest and its anchor is validated.
- A cross-reference written as an absolute `https://learn.netdata.cloud/...` URL is not touched: no anchor validation,
  and no rewrite when the target is later moved or renamed in `map.yaml`. After such a move it reaches the page only
  through the redirect ingest generates for the old route (`../redirects.md`), never directly.
- The generator rewrites a repository-relative `](/...` link to its `https://github.com/netdata/netdata/blob/master/...`
  form (`convert_local_links` in `integrations/gen_integrations.py`, whose `integrations.js` output the page generator
  reads), which is exactly the form `convert_github_links` resolves; an absolute Learn URL passes through both
  untouched. The rule is therefore applied at the source. The audit commands for existing absolute links are in
  `.agents/skills/integrations-lifecycle/how-tos/auditing-metadata-learn-links.md`.

Verified against `netdata/learn @ c3a16edd5ee4dc819976ef162c9afaff4b9b968c` (`local_to_absolute_links` skips links
containing `http`; `convert_github_links` keys on `https://github.com/netdata`) and this repository's
`src/crates/netflow-plugin/integrations/*.md`, whose links carry the GitHub form of the `metadata.yaml` paths.
