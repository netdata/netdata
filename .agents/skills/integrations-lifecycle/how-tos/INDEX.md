# How-tos

Analysis-derived answers to concrete pipeline questions. The rule that keeps this catalog live is in `../SKILL.md`
("Live how-to rule").

| Question | How-to | Audience |
|---|---|---|
| A collector is being retired: what is its whole ownership surface? | [retiring-collector-integration](retiring-collector-integration.md) | authors |
| A section exists in `metadata.yaml` but Website or Learn does not show it | [tracing-missing-published-sections](tracing-missing-published-sections.md) | authors |
| A generated page links to a Learn URL that may have drifted | [auditing-metadata-learn-links](auditing-metadata-learn-links.md) | authors |
| A stock Prometheus profile was added: what happens on the integration catalog? | [prometheus-profile-metadata](prometheus-profile-metadata.md) | authors |
| Adding a new top-level `integration_type` | [adding-new-integration-type](adding-new-integration-type.md) | maintainers |
| Regenerating or changing the NPM catalog (`gen_npm_catalog.py`) | [npm-catalog-generation](npm-catalog-generation.md) | maintainers |

## Adding one

1. Create `how-tos/<slug>.md`: the question in one line, the answer citing files and symbols (never line numbers), and,
   when the work read other repositories, a "How I figured this out" footer naming files and commands so the next reader
   can verify.
2. Add a row above and commit it with the work that prompted the analysis.

Do not add one when an existing guide already covers the question (update that guide), when the answer is a lookup that
needs no analysis, or when it is speculative or tied to a release that will change.
