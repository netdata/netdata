# Alerts, Meta, And Functions: Identity, Cross-References, Live Data

Small families with one thing in common: they are data that other artifacts define (the health configuration, the
product, the Function implementation), copied into `metadata.yaml` so the page and the catalog can show them. Almost
nothing here is authored prose.

Shape rules for every field are in `SKILL.md` and `overview.md` section 1.

## 1. `alerts`: A Mirror Of The Health Configuration

**Question:** which alerts will fire on these charts out of the box?

Rendering (`integrations/templates/alerts.md`): a table with the alert name linked to its configuration file, the
context it watches, and its `info` text. Empty renders "There are no alerts configured by default for this
integration.", which MUST be true.

- Every alert or template in `src/health/health.d/*.conf` whose `on:` context belongs to this collector has a row.
  Every row corresponds to one entry in a conf. Adding, renaming, or removing an alert changes both files in the same
  change.
- `name` is the alert or template name as written in the conf. `metric` is the `on:` context. `info` is the conf's
  `info` line verbatim: same words, same case, same punctuation. `link` is the conf file on the master branch
  (`https://github.com/netdata/netdata/blob/master/src/health/health.d/<file>.conf`). `os` is present only when the
  conf restricts the alert with `os:`, with the same value.
- No alert prose is written here. If the `info` reads badly for the page, fix the conf; the page copies it.
- Rows are ordered as they appear in the conf, so a reader can follow the link and find them in order.

## 2. `meta.monitored_instance`: The Product Identity

**Question:** what product is this page about, where is its home, and how do I find the page?

- `name` is the product name as its vendor writes it ("PostgreSQL", "Amazon CloudWatch", "NGINX Plus"). It is the
  page h1, the catalog row, the slug of the generated file, and the text of every cross-reference; it does not
  contain "metrics", "monitoring", or "collector".
- `link` is the product's official page. It is required unless the source is an operating system or kernel facility
  with no product page of its own (a `/proc` file, a sysctl); then it stays empty and the page shows no link.
- `categories` holds ids from `integrations/categories.yaml`. One primary category; a second only when the collector
  genuinely belongs to both (a database that is also a message queue), never to be found in more places.
- `icon_filename` names an icon file that exists in the dashboard's icon set (served by the UI, not by this repo; the
  pipeline does not validate it). Reuse a filename another integration already uses for the same product or category,
  or add the file to the UI first. A vendor logo when one exists, a generic icon for the category otherwise; never
  another product's icon.

## 3. `meta.keywords`

**Question:** what will an operator type into the search box to find this?

- Five to ten terms: the product name and its aliases and abbreviations, the vendor, the protocol or API, the file or
  service name operators know (`pgsql`, `postgres`, `rds`). All lowercase unless the term is a proper name. A
  multi-service or multi-device collector (cloud monitoring, SNMP) MAY add the service or vendor names operators search
  for; the cap applies to single-product collectors.
- The full product name is always one of them. Not marketing words ("monitoring", "observability", "performance"),
  not generic words the integration name already carries ("server", "database"), not the category.
- Zero keywords is a defect: the page is then reachable only by its name.

## 4. Cross-References

Two fields link integration pages to each other. Both are empty unless a real relationship exists, and when one does,
both sides are filled in the same change.

- `related_resources.integrations.list` names other integrations that monitor the same product from another angle
  (metrics from the API here, logs there; this collector and a Prometheus exporter for the same product). The overview
  template renders them at the end of the Overview as "<Name> can be monitored further using the following other
  integrations:" followed by the linked names only.
- `info_provided_to_referring_integrations.description` is one sentence about what this collector adds for the pages
  that refer to it ("collects runtime metrics from the CloudWatch API"), operator voice, no option names. It reaches
  the generated `integrations.json` but no page: the generator never renders the template that would show it.

## 5. `functions`: Live Data

**Question:** which Functions does this collector offer, what do they need, and what do they return?

Rendering (`integrations/templates/functions.md`): `## Live Data`, the list `description`, then one h3 per entry with
an aspects table (name as `<Module>:<id>`, `require_cloud`, `performance`, `security`, `availability`), h4
Prerequisites (one h5 per entry, or "No additional configuration is required."), h4 Parameters (table), h4 Returns
(`returns.description` and a columns table). Empty renders nothing.

- Every Function the collector registers has an entry, and every entry has a Function in the code. `id`, the
  parameters (name, type, required, default, options), and the return columns (name, type, unit, visibility) mirror
  the implementation verbatim; they are drift checks, not prose.
- The list `description` and each entry's `description` say what the operator gets and when to use it, one
  paragraph each, operator voice. `returns.description` says what one row is.
- `require_cloud`, `performance`, `security`, and `availability` state what the implementation does (a Function that
  runs a query against the monitored system is not "low" performance impact because the author hopes so).
- Prerequisites follow `setup.md` section 2: one operator action each, imperative title, steps, verification.

## 6. Review Questions For This Family

- Does every alert in the health conf for this collector's contexts have a row, and does every row's `info` match the
  conf character for character?
- Is `name` the vendor's spelling, and is `link` the product page (or justifiably empty)?
- Is there one primary category, an existing icon, and five to ten real search terms (plus service or vendor names
  only for a multi-service collector)?
- If `related_resources` is set, does the other integration refer back, and is the referring sentence filled?
