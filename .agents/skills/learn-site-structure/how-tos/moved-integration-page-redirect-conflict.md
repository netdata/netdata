# Resolving "Conflicting redirect identity" when an integration page moves between plugins

## Symptom

Learn ingest aborts in `autogenerateRedirects.py`:

```
ValueError: Conflicting redirect identity /docs/data-collection/ebpf/ebpf-dcstat:
  /docs/collecting-metrics/collectors/operating-systems/ebpf-dcstat
  and https://github.com/netdata/netdata/blob/master/src/collectors/ebpf.plugin/integrations/ebpf_dcstat.md
```

Raised from `combineDictsOverwrite`, called by `main` as
`combineDictsOverwrite(readRedirectsFromFile("netlify.toml"), oldLearn_redirects)`. One legacy Learn URL now
resolves to two different targets, so the run fails rather than silently picking one.

## Why moving a collector between plugins triggers it

The redirect store is anchored to the **GitHub blob URL of the source file**, not to the old Learn URL
(see `redirects.md`, "Indirection: GH-URL anchored"). An integration page's Learn slug, by contrast, is computed
from the integration's `sidebar_label` and category.

When a collector is reimplemented under a different plugin directory, those two identities diverge:

| | old | new |
| --- | --- | --- |
| GH blob URL (redirect key) | `src/collectors/<plugin>/integrations/<name>.md` | `src/collectors/<plugin>/<newplugin>/integrations/<name>.md` |
| Learn slug (redirect value) | `.../operating-systems/<name>` | `.../operating-systems/<name>` — **unchanged** |

Because the metadata keeps the same display name and category, both source files claim the same Learn URL. The
pipeline therefore sees this as a **delete plus add**, not a move, and `addMovedRedirects` cannot pair them: it
matches on `custom_edit_url`, which changed.

This is why the automatic move/rename redirect does NOT cover it. Per `recipes/delete-doc-page.md`, unpublishing
a page is the one case needing manual surgery.

## Fix (learn repo, not the source repo)

Nothing in the source repo needs changing — the integrations generator's full `cleanup()` already removes the
stale page, and integration nodes are inserted into `map.yaml` by ingest's `populate_integrations` step rather
than hand-listed.

In the learn repo:

1. Open `${NETDATA_REPOS_DIR}/learn/LegacyLearnCorrelateLinksWithGHURLs.json`.
2. Find the entry keyed by the **old** blob URL, e.g.
   `https://github.com/netdata/netdata/blob/master/src/collectors/ebpf.plugin/integrations/ebpf_dcstat.md`.
3. Because the new page is a direct content replacement, apply the "redirect to replacement" branch of the
   delete recipe: set the value to the replacement's Learn URL
   (`https://learn.netdata.cloud/docs/collecting-metrics/collectors/operating-systems/ebpf-dcstat`).
   Do NOT drop the entry — dropping it 404s every inbound link to the old page.

Leaving the entry pointed at a source file that no longer exists is what produces the conflict, so editing it is
mandatory rather than cosmetic.

## Sequencing

The conflict appears once the source PR merges and the post-merge regeneration removes the old page. Land the
learn-repo surgery in the same window; otherwise every ingest run (every 3 hours) aborts at this step and no
documentation updates publish at all — the blast radius is the whole site, not just the moved page.

## Generalization

Any collector migration that keeps the integration's display name while changing its source directory hits this.
Check for it whenever a `metadata.yaml` module moves between plugin directories.
