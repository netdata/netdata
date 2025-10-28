"""
Generate the integrations section in COLLECTORS.md from integrations/integrations.js

This script:
- Reads category tree and integrations from integrations.js
- Uses second-level categories (children of 'data-collection') as section headings
- Groups integrations by their section-level category
- Generates markdown tables with integration name, link, and description
"""

from __future__ import annotations

import json
import pathlib
import re
from typing import Any, Dict, List, Optional, Tuple


# =============================================================================
# Data Loading
# =============================================================================

def _extract_json_from_js(js_text: str) -> Tuple[str, str]:
    """Extract categories and integrations JSON from JavaScript file."""
    after_categories = js_text.split("export const categories = ", 1)[1]
    categories_str, after_integrations = after_categories.split("export const integrations = ", 1)
    integrations_str = after_integrations

    def cleanup(s: str) -> str:
        """Remove export statements and trailing semicolons."""
        s = re.split(r"\n\s*export const|\Z", s, maxsplit=1)[0].strip()
        return s.rstrip(';').strip()

    return cleanup(categories_str), cleanup(integrations_str)


def load_catalog(js_path: str = 'integrations/integrations.js') -> Tuple[List[Dict], Dict]:
    """Load and parse categories and integrations from JavaScript file."""
    with open(js_path, 'r', encoding='utf-8') as f:
        js_data = f.read()

    categories_str, integrations_str = _extract_json_from_js(js_data)
    categories = json.loads(categories_str)
    integrations = json.loads(integrations_str)

    return categories, integrations


# =============================================================================
# Category Processing
# =============================================================================

class CategoryMapper:
    """Maps category IDs to titles, parents, and section-level ancestors."""

    def __init__(self, categories: List[Dict[str, Any]]):
        self.id_to_parent: Dict[str, Optional[str]] = {}
        self.id_to_title: Dict[str, str] = {}
        self.section_level_ids: List[str] = []  # Children of 'data-collection'
        self.default_section_ids: List[str] = []  # Section IDs with collector_default=true

        self._build_maps(categories)

    def _build_maps(self, categories: List[Dict[str, Any]]) -> None:
        """Build internal mappings by walking the category tree."""
        default_ids = []

        def walk(nodes: List[Dict[str, Any]], parent: Optional[str]) -> None:
            for node in nodes or []:
                if not isinstance(node, dict):
                    continue

                cid = node.get('id')
                if not cid:
                    continue

                title = node.get('name') or node.get('title') or cid
                self.id_to_parent[cid] = parent
                self.id_to_title[cid] = title

                # Track section-level categories (children of 'data-collection')
                if parent == 'data-collection':
                    self.section_level_ids.append(cid)

                # Track categories with collector_default=true
                if node.get('collector_default') is True:
                    default_ids.append(cid)

                # Recurse into children
                children = node.get('children', [])
                if isinstance(children, list):
                    walk(children, cid)

        walk(categories, parent=None)

        # Roll up default IDs to their section-level ancestors
        for cid in default_ids:
            section = self.get_section_ancestor(cid)
            if section and section not in self.default_section_ids:
                self.default_section_ids.append(section)

    def get_section_ancestor(self, cid: str) -> Optional[str]:
        """Find the section-level ancestor (child of 'data-collection') for a category."""
        cur = cid
        seen = set()

        while cur and cur in self.id_to_parent and cur not in seen:
            seen.add(cur)
            parent = self.id_to_parent.get(cur)

            if parent == 'data-collection':
                return cur
            if parent is None:
                return None

            cur = parent

        return None


# =============================================================================
# Text Processing
# =============================================================================

def extract_first_sentence(text: str) -> str:
    """Extract the first sentence from text (up to the first period)."""
    if not text:
        return text

    # Match first sentence ending with period followed by space/newline
    match = re.match(r'^(.*?\.)\s', text)
    if match:
        return match.group(1).strip()

    # If text ends with period, use all of it
    if text.endswith('.'):
        return text.strip()

    # No period found - use all text
    return text.strip()


def extract_description_from_overview(overview: str) -> Optional[str]:
    """Extract first substantial paragraph from markdown overview section."""
    # Split by ## Overview heading
    parts = overview.split('## Overview', 1)
    if len(parts) <= 1:
        return None

    # Get text after ## Overview
    text_after = parts[1].strip()

    # Split into lines and find first substantial paragraph
    lines = text_after.split('\n')
    paragraph = []

    for line in lines:
        line = line.strip()

        # Skip empty lines at start
        if not line and not paragraph:
            continue

        # Skip metadata lines (Plugin:, Module:, headings)
        if line.startswith(('#', 'Plugin:', 'Module:')):
            if paragraph:  # Stop if we already have content
                break
            continue

        # Collect paragraph lines
        if line:
            paragraph.append(line)
        elif paragraph:  # Empty line after content = end of paragraph
            break

    if not paragraph:
        return None

    text = ' '.join(paragraph)
    first_sentence = extract_first_sentence(text)

    # Normalize whitespace
    return re.sub(r'\s+', ' ', first_sentence) if first_sentence else None


def get_integration_description(integ: Dict[str, Any]) -> str:
    """Get user-friendly description for an integration."""
    # Try overview markdown first
    overview = integ.get('overview')
    if isinstance(overview, str) and overview.strip():
        desc = extract_description_from_overview(overview)
        if desc:
            return desc

    # Fallback to monitored_instance.description
    mi = integ.get('meta', {}).get('monitored_instance', {})
    if isinstance(mi, dict):
        desc = mi.get('description')
        if isinstance(desc, str) and desc.strip():
            first_sentence = extract_first_sentence(desc.strip())
            if first_sentence:
                return re.sub(r'\s+', ' ', first_sentence)

    # Generic fallback
    name = (mi.get('name') if isinstance(mi, dict) else None) or integ.get('name') or 'this integration'
    return f"Monitor {name}"


# =============================================================================
# Link Generation
# =============================================================================

def to_slug(text: str) -> str:
    """Convert text to URL-friendly slug."""
    return text.lower().replace(' ', '_').replace('/', '-').replace('(', '').replace(')', '')


def get_integration_doc_link(integ: Dict[str, Any], display_name: str) -> str:
    """Generate documentation link for an integration.

    Example:
        edit_link: .../collector/rspamd/metadata.yaml
        output:    .../collector/rspamd/integrations/rspamd.md
    """
    edit_link = integ.get('edit_link', '') if isinstance(integ, dict) else ''
    if not edit_link:
        return ''

    base = edit_link.replace('metadata.yaml', '')
    slug = to_slug(display_name)

    return f"{base}integrations/{slug}.md"


# =============================================================================
# Section Collection
# =============================================================================

def collect_integrations_by_section(
        categories: List[Dict[str, Any]],
        integrations: Dict[str, Any]
) -> List[Tuple[str, List[Tuple[str, str, str]]]]:
    """Group integrations by their section-level category.

    Returns:
        List of (section_title, [(name, link, description), ...])
    """
    mapper = CategoryMapper(categories)

    # Determine section order (Linux first, Other last)
    ordered_sections = _get_ordered_sections(mapper)

    # Initialize buckets for each section
    per_section = {cid: [] for cid in ordered_sections}
    other_id = _find_other_category_id(mapper)
    if other_id:
        per_section[other_id] = []
    other_bucket = []

    # Process each integration
    for integ in _iterate_integrations(integrations):
        entry = _process_integration(integ, mapper, ordered_sections, other_id)
        if not entry:
            continue

        name, link, desc, target_sections = entry

        if not target_sections:
            other_bucket.append((name, link, desc))
        else:
            for sec in target_sections:
                if sec in per_section or sec == other_id:
                    per_section[sec].append((name, link, desc))

    # Build final section list
    return _build_section_list(mapper, ordered_sections, per_section, other_id, other_bucket)


def _get_ordered_sections(mapper: CategoryMapper) -> List[str]:
    """Get section IDs in order: Linux first, others alphabetically, Other excluded."""
    linux_id = None
    other_sections = []

    for cid in mapper.section_level_ids:
        if 'linux-systems' in cid.lower():
            linux_id = cid
        elif mapper.id_to_title.get(cid, '').lower() != 'other':
            other_sections.append(cid)

    ordered = []
    if linux_id:
        ordered.append(linux_id)
    ordered.extend(other_sections)

    return ordered


def _find_other_category_id(mapper: CategoryMapper) -> Optional[str]:
    """Find the 'Other' category ID if it exists."""
    for cid in mapper.section_level_ids:
        if mapper.id_to_title.get(cid, '').lower() == 'other':
            return cid
    return None


def _iterate_integrations(integrations: Any):
    """Yield integration objects from dict or list."""
    if isinstance(integrations, dict):
        for integ in integrations.values():
            if isinstance(integ, dict):
                yield integ
    elif isinstance(integrations, list):
        for integ in integrations:
            if isinstance(integ, dict):
                yield integ


def _process_integration(
        integ: Dict[str, Any],
        mapper: CategoryMapper,
        ordered_sections: List[str],
        other_id: Optional[str]
) -> Optional[Tuple[str, str, str, List[str]]]:
    """Process a single integration and determine its target sections.

    Returns:
        (name, link, description, target_section_ids) or None if invalid
    """
    # Get integration name
    mi = integ.get('meta', {}).get('monitored_instance', {})
    name = (mi.get('name') if isinstance(mi, dict) else None) or integ.get('name')
    if not isinstance(name, str) or not name.strip():
        return None

    # Generate link and description
    link = get_integration_doc_link(integ, name)
    desc = get_integration_description(integ)

    # Get categories
    cats = mi.get('categories') if isinstance(mi, dict) else None
    if isinstance(cats, str):
        cats = [cats]
    if not isinstance(cats, list):
        cats = []

    # Determine target sections
    if not cats:
        # Use default sections
        target_sections = list(mapper.default_section_ids)
    else:
        # Roll up each category to its section-level ancestor
        target_sections = []
        for cid in cats:
            section = mapper.get_section_ancestor(cid) if isinstance(cid, str) else None
            if section and section not in target_sections:
                # Only include if it's in our ordered sections or is the other_id
                if section in ordered_sections or section == other_id:
                    target_sections.append(section)

    return (name, link, desc, target_sections)


def _build_section_list(
        mapper: CategoryMapper,
        ordered_sections: List[str],
        per_section: Dict[str, List[Tuple[str, str, str]]],
        other_id: Optional[str],
        other_bucket: List[Tuple[str, str, str]]
) -> List[Tuple[str, List[Tuple[str, str, str]]]]:
    """Build final list of sections with their sorted integrations."""
    sections = []

    # Add ordered sections
    for cid in ordered_sections:
        items = per_section.get(cid, [])
        if items:
            items.sort(key=lambda t: t[0].lower())
            sections.append((mapper.id_to_title.get(cid, cid), items))

    # Add "Other" section last
    if other_id and per_section.get(other_id):
        items = per_section[other_id]
        items.sort(key=lambda t: t[0].lower())
        sections.append((mapper.id_to_title.get(other_id, "Other"), items))
    elif other_bucket:
        other_bucket.sort(key=lambda t: t[0].lower())
        sections.append(("Other", other_bucket))

    return sections


# =============================================================================
# Markdown Rendering
# =============================================================================

def render_header() -> str:
    """Render the header section with marketing content and navigation."""
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


def _render_tech_navigation() -> str:
    """Render the 'Find Your Technology' quick navigation section."""
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
    """Render the 'Beyond the 850+ integrations' section."""
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

    collector_lines = []
    for collector in generic_collectors:
        collector_lines.append(f"- **[{collector['name']}]({collector['link']})** - {collector['description']}")

    collectors_section = "\n".join(collector_lines)

    return f"""## Beyond the 850+ integrations

Netdata can monitor virtually any application through generic collectors:

{collectors_section}

Need a dedicated integration? [Submit a feature request](https://github.com/netdata/netdata/issues/new/choose) on GitHub.
"""


def render_tables(sections: List[Tuple[str, List[Tuple[str, str, str]]]]) -> str:
    """Render markdown tables for all sections."""
    lines = []

    for title, items in sections:
        if not items:
            continue

        lines.append(f"### {title}\n\n")
        lines.append("| Integration | Description |\n|-------------|-------------|\n")

        for name, link, desc in items:
            # Escape pipe characters in description
            desc = desc.replace('|', '\\|')
            lines.append(f"| [{name}]({link}) | {desc} |\n")

        lines.append("\n")

    return ''.join(lines)


# =============================================================================
# Main Execution
# =============================================================================

def generate_collectors_md() -> None:
    """Generate COLLECTORS.md from integrations.js."""
    # Load data
    categories, integrations = load_catalog()

    # Process integrations
    sections = collect_integrations_by_section(categories, integrations)

    # Render markdown
    header = render_header()
    tables = render_tables(sections)
    content = header + "## Available Data Collection Integrations\n\n" + tables

    # Write to file atomically
    outfile = pathlib.Path("./src/collectors/COLLECTORS.md")
    outfile.parent.mkdir(parents=True, exist_ok=True)

    tmp = outfile.with_suffix(outfile.suffix + ".tmp")
    tmp.write_text(content.rstrip('\n') + "\n", encoding='utf-8')
    tmp.replace(outfile)


if __name__ == '__main__':
    generate_collectors_md()
