# Do links inside metadata.yaml-generated integration card descriptions follow the same repo-relative-link rule as docs/ pages?

**Yes.** `meta.description` / troubleshooting `description` text in a
collector's or plugin's `metadata.yaml` (e.g.
`src/crates/netflow-plugin/metadata.yaml`) is rendered into a per-integration
`.md` file carrying the `INTEGRATION_MARKER`. Those files are picked up at
ingest step 6, "Populate integrations" (`ingest.py:2832`, see `pipeline.md`),
which replaces `integration_placeholder` nodes in the `map.yaml` tree with
real rows for each discovered integration page. From that point on an
integration card is an ordinary `to_publish` entry and flows through the same
steps 7-12 as any hand-authored `docs/` page -- including step 10's
`local_to_absolute_links` / step 12's `convert_github_links`
(`ingest.py:2916-2930`), which rewrite repo-relative markdown links to final
Learn URLs and validate header anchors, accumulating broken-anchor warnings.

**Implication:** a cross-reference written in `metadata.yaml` as an already-resolved
absolute `https://learn.netdata.cloud/...` URL is *not* touched by this rewrite
step (nothing to rewrite -- it already looks like a final URL), so it bypasses
anchor validation and does not get auto-corrected if the target page is later
renamed/moved in `map.yaml` (unlike a repo-relative link, which re-resolves
against the current `map.yaml` on every ingest run). Per `mapping.md`'s
"Inter-page linking" section, the objectively correct form for any
integration-card cross-reference to another Learn page is the repo-relative
`/docs/<repo-rel-path>.md[#anchor]` form, not an absolute `learn.netdata.cloud`
URL -- even though the absolute form happens to render correctly today as long
as the slug hasn't changed since it was hand-written.

**Confirmed incident:** `src/crates/netflow-plugin/metadata.yaml` has (at the
time of this note) 49 absolute `learn.netdata.cloud` cross-references across
its troubleshooting/description text, all resolving correctly against the
current `map.yaml` slugs, but none of them repo-relative and thus none
anchor-validated or rename-safe. Two were converted to repo-relative form
during PR #23239's review; the remaining ones are an unconverted pre-existing
pattern, not something introduced by that PR.

## How I figured this out

- `mapping.md`'s "Inter-page linking" section states the repo-relative rule
  and gives the "Correct"/"Wrong" example.
- `pipeline.md` step 6 ("Populate integrations") confirms integration `.md`
  files are inserted into the `map.yaml`-driven publish tree before steps
  7-12 run.
- `pipeline.md` steps 10-12 document `local_to_absolute_links` /
  `convert_github_links` as the link-rewrite + anchor-validation mechanism
  that only fires on repo-relative/GitHub-view links, not on links that are
  already in final `learn.netdata.cloud` form.
- Verified in `src/crates/netflow-plugin/metadata.yaml`: the rendered output
  in `src/crates/netflow-plugin/integrations/*.md` reproduces each
  `description:` field's link text byte-for-byte, confirming the generator
  does not itself rewrite or validate these links -- whatever is written in
  `metadata.yaml` is exactly what ships to the ingest pipeline.
