"""
Generate the integrations section in COLLECTORS.md from integrations/integrations.js
with per-category **tables** matching desired-COLLECTORS.md style.
Key behavior:
- Use **top-level categories as section headings**. Any child category IDs on integrations
  are rolled up to their top-level parent section.
- Read categories from meta.monitored_instance.categories (array of strings).
- If an integration has no categories, assign categories where any ancestor has collector_default=true,
  rolled up to its top-level parent(s).
- Render Markdown tables: | Integration | Description |.
"""

from __future__ import annotations

import json
import pathlib
import re
from typing import Any, Dict, List, Tuple, Iterable, Optional


def _extract_json_blobs(js_text: str) -> Tuple[str, str]:
    after_categories = js_text.split("export const categories = ", 1)[1]
    categories_str, after_integrations = after_categories.split("export const integrations = ", 1)
    integrations_str = after_integrations
    def _cleanup(s: str) -> str:
        s = re.split(r"\n\s*export const|\Z", s, maxsplit=1)[0].strip()
        if s.endswith(';'):
            s = s[:-1]
        return s.strip()
    return _cleanup(categories_str), _cleanup(integrations_str)


def _load_catalog(js_path: str = 'integrations/integrations.js'):
    with open(js_path, 'r', encoding='utf-8') as f:
        js_data = f.read()
    categories_str, integrations_str = _extract_json_blobs(js_data)
    categories = json.loads(categories_str)  # expected array tree
    integrations = json.loads(integrations_str)  # expected dict map
    return categories, integrations


def _build_category_maps(categories: List[Dict[str, Any]]):
    """Build maps: id->title, id->parent_id, section_level list (ordered), and defaults rolled to section-level.

    We use the children of 'data-collection' as section headings (second-level categories).

    """
    id_to_parent: Dict[str, Optional[str]] = {}
    id_to_title: Dict[str, str] = {}
    section_level_ids: List[str] = []  # Second-level categories under data-collection
    default_ids: List[str] = []  # category ids where collector_default==true

    def walk(nodes: Iterable[Dict[str, Any]], parent: Optional[str], depth: int = 0):
        for node in nodes or []:
            if not isinstance(node, dict):
                continue
            cid = node.get('id')
            title = node.get('name') or node.get('title') or (cid or '')
            if not cid:
                continue
            id_to_parent[cid] = parent
            id_to_title[cid] = title

            # If this is a child of 'data-collection', it's a section heading
            if parent == 'data-collection':
                section_level_ids.append(cid)

            if node.get('collector_default') is True:
                default_ids.append(cid)

            children = node.get('children') or []
            if isinstance(children, list) and children:
                walk(children, cid, depth + 1)

    walk(categories if isinstance(categories, list) else [], parent=None, depth=0)

    # Find section-level ancestor (child of data-collection)
    def section_ancestor(cid: str) -> Optional[str]:
        # Walk up until we find a category whose parent is 'data-collection'
        cur = cid
        seen = set()
        while cur is not None and cur in id_to_parent and cur not in seen:
            seen.add(cur)
            parent = id_to_parent.get(cur)
            if parent == 'data-collection':
                return cur
            if parent is None:
                # If we reach the top without finding data-collection, this might be a top-level category
                return None
            cur = parent
        return None

    # Roll default ids to their section-level parents (unique)
    default_section_level = []
    for d in default_ids:
        section = section_ancestor(d)
        if section and section not in default_section_level:
            default_section_level.append(section)

    return id_to_title, id_to_parent, section_level_ids, default_section_level, section_ancestor


def _desc_for_integration(integ: Dict[str, Any]) -> str:
    """Generate user-friendly description for integration."""
    overview = integ.get('overview')
    if isinstance(overview, dict):
        dc = overview.get('data_collection')
        if isinstance(dc, dict):
            md = dc.get('metrics_description')
            if isinstance(md, str) and md.strip():
                return re.sub(r"\s+", " ", md.strip())
    mi = integ.get('meta', {}).get('monitored_instance', {})
    if isinstance(mi, dict):
        mi_desc = mi.get('description')
        if isinstance(mi_desc, str) and mi_desc.strip():
            return re.sub(r"\s+", " ", mi_desc.strip())
    for key in ('short_description', 'description', 'summary'):
        v = integ.get(key)
        if isinstance(v, str) and v.strip():
            return re.sub(r"\s+", " ", v.strip())
    name = (mi.get('name') if isinstance(mi, dict) else None) or integ.get('name') or 'Integration'
    return f"Metrics for {name}"


def _to_slug(text: str) -> str:
    """Convert a string to a slug suitable for URLs and anchors."""
    return text.lower().replace(' ', '_').replace('/', '-').replace('(', '').replace(')', '')

def _doc_link(integ: Dict[str, Any], display_name: str) -> str:
    base = (integ.get('edit_link') or '') if isinstance(integ, dict) else ''
    base = base.replace('metadata.yaml', '')
    slug = _to_slug(display_name)
    return f"{base}integrations/{slug}.md"


def _collect_sections(categories: List[Dict[str, Any]], integrations: Dict[str, Any]):
    id_to_title, id_to_parent, section_level_ids, default_section_level, section_ancestor = _build_category_maps(categories)

    # Custom section order: Linux Systems first, then others, but exclude "Other" category
    ordered_sections = []
    linux_id = None
    other_id = None

    for cid in section_level_ids:
        if 'linux-systems' in cid.lower():
            linux_id = cid
        elif id_to_title.get(cid, '').lower() == 'other':
            other_id = cid

    if linux_id:
        ordered_sections.append(linux_id)

    for cid in section_level_ids:
        if cid != linux_id and cid != other_id:
            ordered_sections.append(cid)

    # Prepare buckets for section-level categories
    per_section: Dict[str, List[Tuple[str, str, str]]] = {cid: [] for cid in ordered_sections}
    if other_id:
        per_section[other_id] = []
    other_bucket: List[Tuple[str, str, str]] = []

    items = integrations.items() if isinstance(integrations, dict) else enumerate(integrations if isinstance(integrations, list) else [])
    for _key, integ in items:
        if not isinstance(integ, dict):
            continue
        mi = integ.get('meta', {}).get('monitored_instance', {})
        name = (mi.get('name') if isinstance(mi, dict) else None) or integ.get('name')
        if not isinstance(name, str) or not name.strip():
            continue
        link = _doc_link(integ, name)
        desc = _desc_for_integration(integ)

        cats = []
        if isinstance(mi, dict):
            cats = mi.get('categories') or []
        if isinstance(cats, str):
            cats = [cats]
        if not isinstance(cats, list):
            cats = []

        if not cats:
            # use default section-level categories
            target_sections = list(default_section_level) if default_section_level else []
        else:
            # roll each category id to its section-level ancestor
            target_sections = []
            for cid in cats:
                section = section_ancestor(cid) if isinstance(cid, str) else None
                if section and (section in per_section or section == other_id) and section not in target_sections:
                    target_sections.append(section)

        if not target_sections:
            other_bucket.append((name, link, desc))
        else:
            for sec in target_sections:
                if sec in per_section or sec == other_id:
                    per_section[sec].append((name, link, desc))

    # Build ordered sections with non-empty tables
    sections: List[Tuple[str, List[Tuple[str, str, str]]]] = []
    for cid in ordered_sections:
        items = per_section.get(cid, [])
        if items:
            items.sort(key=lambda t: t[0].lower())
            sections.append((id_to_title.get(cid, cid), items))

    # Add "Other" section last - either from other_bucket or from the Other category
    if other_id and per_section.get(other_id):
        items = per_section[other_id]
        items.sort(key=lambda t: t[0].lower())
        sections.append((id_to_title.get(other_id, "Other"), items))
    elif other_bucket:
        other_bucket.sort(key=lambda t: t[0].lower())
        sections.append(("Other", other_bucket))

    return sections


def _render_tech_navigation() -> str:
    """Render the 'Find Your Technology' navigation section."""
    # Configuration for "Find Your Technology" section
    tech_categories = [
        {
            "title": "Cloud & Infrastructure:",
            "items": [
                ("AWS", "#cloud-provider-managed"),
                ("Azure", "#cloud-provider-managed"),
                ("GCP", "#cloud-provider-managed"),
                ("Kubernetes", "#kubernetes"),
                ("Docker", "#containers-and-vms"),
                ("VMware", "#containers-and-vms"),
            ]
        },
        {
            "title": "Databases & Caching:",
            "items": [
                ("MySQL", "#databases"),
                ("PostgreSQL", "#databases"),
                ("MongoDB", "#databases"),
                ("Redis", "#databases"),
                ("Elasticsearch", "#search-engines"),
                ("Oracle", "#databases"),
            ]
        },
        {
            "title": "Web & Application:",
            "items": [
                ("NGINX", "#web-servers-and-web-proxies"),
                ("Apache", "#web-servers-and-web-proxies"),
                ("HAProxy", "#web-servers-and-web-proxies"),
                ("Tomcat", "#web-servers-and-web-proxies"),
                ("PHP-FPM", "#web-servers-and-web-proxies"),
            ]
        },
        {
            "title": "Message Queues:",
            "items": [
                ("Kafka", "#message-brokers"),
                ("RabbitMQ", "#message-brokers"),
                ("ActiveMQ", "#message-brokers"),
                ("NATS", "#message-brokers"),
                ("Pulsar", "#message-brokers"),
            ]
        },
        {
            "title": "Operating Systems:",
            "items": [
                ("Linux", "#linux-systems"),
                ("Windows", "#windows-systems"),
                ("macOS", "#macos-systems"),
                ("FreeBSD", "#freebsd"),
            ]
        },
    ]

    # Build "Find Your Technology" section
    tech_lines = []
    for category in tech_categories:
        links = " • ".join([f"[{name}]({anchor})" for name, anchor in category["items"]])
        tech_lines.append(f"**{category['title']}**\n{links}\n")

    tech_section = "\n".join(tech_lines)

    return f"""### Find Your Technology

**Select your primary infrastructure to jump directly to relevant integrations:**

{tech_section}
**Don't see what you need?** We support [Prometheus endpoints](#generic-data-collection), [SNMP devices](#generic-data-collection), [StatsD](#beyond-the-850-integrations), and [custom data sources](#generic-data-collection).
"""


def _render_generic_collectors() -> str:
    """Render the 'Beyond the 850+ integrations' section with generic collectors."""
    # Configuration for generic collectors
    generic_collectors = [
        {
            "name": "Prometheus collector",
            "link": "/src/go/plugin/go.d/collector/prometheus/README.md",
            "description": "Any application exposing Prometheus metrics"
        },
        {
            "name": "StatsD collector",
            "link": "/src/collectors/statsd.plugin/README.md",
            "description": "Applications instrumented with [StatsD](https://blog.netdata.cloud/introduction-to-statsd/)"
        },
        {
            "name": "Pandas collector",
            "link": "/src/collectors/python.d.plugin/pandas/README.md",
            "description": "Structured data from CSV, JSON, XML, and more"
        },
    ]

    # Build generic collectors section
    collector_lines = []
    for collector in generic_collectors:
        collector_lines.append(f"- **[{collector['name']}]({collector['link']})** - {collector['description']}")

    collectors_section = "\n".join(collector_lines)

    return f"""## Beyond the 850+ integrations

Netdata can monitor virtually any application through generic collectors:

{collectors_section}

Need a dedicated integration? [Submit a feature request](https://github.com/netdata/netdata/issues/new/choose) on GitHub.
"""


def _render_header() -> str:
    """Render the marketing header and navigation sections."""
    tech_nav = _render_tech_navigation()
    generic_section = _render_generic_collectors()

    return f"""# Monitor anything with Netdata

**850+ integrations. Zero configuration. Deploy anywhere.**

Netdata uses collectors to help you gather metrics from your favorite applications and services and view them in real-time, interactive charts. The following list includes all the integrations where Netdata can gather metrics from.

Learn more about [how collectors work](/src/collectors/README.md), and then learn how to [enable or configure](/src/collectors/REFERENCE.md#enable-or-disable-collectors-and-plugins) a specific collector.

### Why Teams Choose Us

- ✅ **850+ integrations** automatically discovered and configured
- ✅ **Zero configuration** required - monitors start collecting data immediately
- ✅ **No vendor lock-in** - Deploy anywhere, own your data
- ✅ **1-second resolution** - Real-time visibility, not delayed averages
- ✅ **Flexible deployment** - On-premise, cloud, or hybrid

{tech_nav}

{generic_section}

"""


def _render_tables(sections: List[Tuple[str, List[Tuple[str, str, str]]]]) -> str:
    lines: List[str] = []
    for title, items in sections:
        if not items:
            continue

        # Convert title to anchor-compatible format
        anchor = title.lower().replace(' ', '-').replace('/', '-').replace('(', '').replace(')', '')
        lines.append(f"### {title}\n\n")
        lines.append("| Integration | Description |\n|-------------|-------------|\n")

        for name, link, desc in items:
            desc = desc.replace('|', '\\|')
            lines.append(f"| [{name}]({link}) | {desc} |\n")
        lines.append("\n")
    return ''.join(lines)


def main() -> None:
    categories, integrations = _load_catalog()
    sections = _collect_sections(categories, integrations)

    header = _render_header()
    tables = _render_tables(sections)
    md = header + "## Available Data Collection Integrations\n\n" + tables

    outfile = pathlib.Path("./src/collectors/COLLECTORS.md")
    txt = outfile.read_text(encoding='utf-8')

    # Find the start of the content to replace
    if "## Available Data Collection Integrations" in txt:
        pre = txt.split("## Available Data Collection Integrations")[0]
    elif "# Monitor anything with Netdata" in txt:
        # If the header exists, keep only what's before it
        pre = txt.split("# Monitor anything with Netdata")[0]
    else:
        # Otherwise keep everything before the marker
        pre = txt.split("## Add your application to Netdata")[0] if "## Add your application to Netdata" in txt else ""

    new_txt = pre.rstrip() + "\n\n" + md
    outfile.write_text(new_txt.rstrip('\n') + "\n", encoding='utf-8')


if __name__ == '__main__':
    main()
