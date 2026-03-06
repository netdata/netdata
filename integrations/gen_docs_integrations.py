#!/usr/bin/env python3
import argparse
import json
import re
import shutil
import sys
from pathlib import Path

from jinja2 import Environment, FileSystemLoader, select_autoescape

# Registry used to decide which README.md should symlink to which generated file
symlink_dict = {}

# Mapping of integration id → output file path (repo-relative), populated by write_to_file()
id_to_path = {}

TEMPLATE_PATH = Path(__file__).parent / "templates"
_jinja_env = None


# -----------------------------
# FS utilities
# -----------------------------
def cleanup(only_base_paths=None):
    """
    Clean generated /integrations folders.
    - If only_base_paths is provided (list of base dirs), clean ONLY those.
    - Otherwise, do a full cleanup (legacy behavior).
    """
    targets = [
        "src/go/plugin/go.d/collector",
        "src/go/plugin/ibm.d/modules",
        "src/go/plugin/scripts.d/modules",
        "src/crates/netdata-otel",
        "src/collectors",
        "src/exporting",
        "integrations/cloud-notifications",
        "integrations/logs",
        "integrations/cloud-authentication",
    ]
    bases = only_base_paths if only_base_paths else targets
    for base in bases:
        for p in Path(base).glob("**/integrations"):
            shutil.rmtree(p, ignore_errors=True)


def normalize_markdown(md: str) -> str:
    md = re.sub(r'\{% details open=true summary="(.*?)" %\}', r'<details open><summary>\1</summary>\n', md)
    md = re.sub(r'\{% details summary="(.*?)" %\}', r'<details><summary>\1</summary>\n', md)
    md = md.replace("{% /details %}", "</details>\n")
    return md


def strfy(value):
    if isinstance(value, bool):
        return "yes" if value else "no"
    if isinstance(value, str):
        return " ".join([v.strip() for v in value.strip().split("\n") if v]).replace("|", "/")
    if value is None:
        return ""
    return value


def anchorfy(value):
    if value is None:
        return ""
    anchor = str(value).strip().lower()
    anchor = re.sub(r"[^a-z0-9]+", "-", anchor)
    anchor = re.sub(r"-{2,}", "-", anchor).strip("-")
    return anchor


def first_sentence(text: str) -> str:
    if not text:
        return ""
    flattened = " ".join(part.strip() for part in text.splitlines() if part.strip())
    if not flattened:
        return ""
    match = re.match(r"^(.*?[.!?])(\s|$)", flattened)
    if match:
        return match.group(1).strip()
    return flattened


def get_jinja_env():
    global _jinja_env
    if _jinja_env is None:
        _jinja_env = Environment(
            loader=FileSystemLoader(TEMPLATE_PATH),
            autoescape=select_autoescape(),
            block_start_string="[%", block_end_string="%]",
            variable_start_string="[[", variable_end_string="]]",
            comment_start_string="[#", comment_end_string="#]",
            trim_blocks=True, lstrip_blocks=True,
        )
        _jinja_env.globals.update(strfy=strfy)
    return _jinja_env


def function_slug(func_id: str) -> str:
    return anchorfy(func_id or "")


def integration_doc_slug(integration: dict) -> str:
    monitored = integration.get("meta", {}).get("monitored_instance", {})
    name = monitored.get("name") or integration.get("id") or "integration"
    return clean_string(name)


def function_tile_filename(integration_slug: str, func_slug: str) -> str:
    return f"{integration_slug}-{func_slug}.md"


def build_repo_root_integration_path(base_path: str, integration_slug: str) -> str:
    return f"/{base_path}/integrations/{integration_slug}.md"


def build_repo_root_function_path(base_path: str, integration_slug: str, func_slug: str) -> str:
    filename = function_tile_filename(integration_slug, func_slug)
    return f"/{base_path}/integrations/functions/{filename}"


def function_learn_rel_path(collector_learn_rel_path: str) -> str:
    if not collector_learn_rel_path:
        return "Live View"
    if collector_learn_rel_path == "Collecting Metrics":
        return "Live View"
    if collector_learn_rel_path.startswith("Collecting Metrics/"):
        return collector_learn_rel_path.replace("Collecting Metrics/", "Live View/", 1)
    return collector_learn_rel_path


def function_keywords(integration: dict, func: dict):
    keywords = func.get("keywords")
    if keywords:
        return keywords
    return integration.get("meta", {}).get("keywords")


def render_functions_index(integration: dict, base_path: str) -> str:
    functions = integration.get("functions_data")
    if not functions:
        return integration.get("functions", "")

    function_list = functions.get("list")
    if not function_list:
        return integration.get("functions", "")

    integration_slug = integration_doc_slug(integration)
    lines = ["## Functions", ""]
    description = functions.get("description", "")
    if description:
        lines.append(description.strip())
        lines.append("")

    rendered_links = 0
    for func in function_list:
        slug = function_slug(func.get("id"))
        if not slug:
            continue
        rendered_links += 1
        lines.append(f'<a id="{slug}"></a>')
        lines.append(f"### {func.get('name', slug)}")
        lines.append("")
        summary = first_sentence(func.get("description", ""))
        if summary:
            lines.append(summary)
            lines.append("")
        lines.append(
            f"[Full documentation]({build_repo_root_function_path(base_path, integration_slug, slug)})"
        )
        lines.append("")

    if rendered_links == 0:
        return integration.get("functions", "")

    return "\n".join(lines).strip()


def render_function_tile(
    integration: dict,
    func: dict,
    meta_yaml: str,
    learn_rel_path: str,
    integration_doc_path: str,
    keywords,
) -> str:
    template = get_jinja_env().get_template("function_tile.md")
    return template.render(
        entry=integration,
        func=func,
        meta_yaml=meta_yaml,
        learn_rel_path=learn_rel_path,
        integration_doc_path=integration_doc_path,
        keywords=keywords,
    )


def write_function_tiles(base_path: str, integration: dict, meta_yaml: str, learn_rel_path: str):
    functions = integration.get("functions_data")
    if not functions or not functions.get("list"):
        return

    tile_learn_rel_path = function_learn_rel_path(learn_rel_path)
    int_slug = integration_doc_slug(integration)
    integration_doc_path = build_repo_root_integration_path(base_path, int_slug)
    functions_dir = Path(base_path) / "integrations" / "functions"
    functions_dir.mkdir(parents=True, exist_ok=True)

    for func in functions["list"]:
        func_slug = function_slug(func.get("id"))
        if not func_slug:
            raise ValueError(f"Function id is missing or invalid for integration '{integration.get('id')}'.")

        filename = function_tile_filename(int_slug, func_slug)
        outfile = functions_dir / filename
        rendered = render_function_tile(
            integration,
            func,
            meta_yaml,
            tile_learn_rel_path,
            integration_doc_path,
            function_keywords(integration, func),
        )
        rendered_clean = normalize_markdown(rendered)

        if outfile.exists():
            existing = outfile.read_text(encoding="utf-8")
            if existing != rendered_clean:
                raise ValueError(
                    f"Conflicting function tile filename '{filename}' for collector path '{base_path}'. "
                    f"File '{outfile}' already exists with different content."
                )
            continue

        outfile.write_text(rendered_clean, encoding="utf-8")


def clean_and_write(md: str, path: Path):
    """
    Convert custom markers to HTML/plain text for GitHub-rendered .md files.
    relatedResource tags are left as-is here; they are resolved in a post-pass
    once id_to_path is fully populated.
    """
    path.write_text(normalize_markdown(md), encoding="utf-8")


def resolve_related_links():
    """
    Post-process all written files: convert relatedResource tags to markdown links.
    Must be called after all files are written and id_to_path is fully populated.
    """
    for fpath in id_to_path.values():
        p = Path(fpath)
        if not p.exists():
            continue
        md = p.read_text(encoding="utf-8")
        if '{% relatedResource' not in md:
            continue

        def _resolve(m):
            rid = m.group(1)
            name = m.group(2)
            target = id_to_path.get(rid)
            if target:
                return f'[{name}](/{target})'
            return name

        md = re.sub(r'\{% relatedResource id="([^"]*)" %\}(.*?)\{% /relatedResource %\}', _resolve, md)
        p.write_text(md, encoding="utf-8")


def expected_output_path_for_integration(integration: dict):
    """
    Best-effort path prediction used to resolve relatedResource links even in
    targeted (-c) generation mode where only a subset of files is rewritten.
    """
    iid = integration.get("id")
    itype = integration.get("integration_type")
    edit_link = integration.get("edit_link")
    if not iid or not itype or not edit_link:
        return None

    meta_yaml = edit_link.replace("blob", "edit")
    base = Path(build_path(meta_yaml))
    meta = integration.get("meta", {})

    if itype == "collector":
        name = meta.get("monitored_instance", {}).get("name")
        if not name:
            return None
        return str(base / "integrations" / f"{clean_string(name)}.md")

    if itype in {"exporter", "cloud_notification", "logs", "authentication"}:
        name = meta.get("name")
        if not name:
            return None
        return str(base / "integrations" / f"{clean_string(name)}.md")

    if itype == "agent_notification":
        return str(base / "README.md")

    return None


def seed_id_to_expected_paths(integrations):
    """
    Seed id_to_path with deterministic expected paths for all integrations.
    Actual writes later overwrite these entries with exact output paths.
    """
    for integration in integrations:
        iid = integration.get("id")
        if not iid:
            continue
        expected = expected_output_path_for_integration(integration)
        if expected:
            id_to_path[iid] = expected


def build_path(meta_yaml_link: str) -> str:
    """
    Convert GitHub edit link to local repo path (without trailing /metadata.yaml).
    """
    return (
        meta_yaml_link.replace("https://github.com/netdata/", "")
        .split("/", 1)[1]
        .replace("edit/master/", "")
        .replace("/metadata.yaml", "")
    )


# -----------------------------
# Content builders
# -----------------------------
def add_custom_edit_url(markdown_string: str, meta_yaml_link: str, sidebar_label_string: str,
                        mode: str = "default") -> str:
    """
    Inject custom_edit_url into the metadata header.
    """
    if mode == "default":
        path_to_md_file = f"{meta_yaml_link.replace('/metadata.yaml', '')}/integrations/{clean_string(sidebar_label_string)}"
    elif mode in ("cloud-notification", "logs", "cloud-authentication"):
        path_to_md_file = meta_yaml_link.replace("metadata.yaml", f"integrations/{clean_string(sidebar_label_string)}")
    elif mode == "agent-notification":
        path_to_md_file = meta_yaml_link.replace("metadata.yaml", "README")
    else:
        # safe fallback
        path_to_md_file = f"{meta_yaml_link.replace('/metadata.yaml', '')}/integrations/{clean_string(sidebar_label_string)}"

    return markdown_string.replace(
        "<!--startmeta", f"<!--startmeta\ncustom_edit_url: \"{path_to_md_file}.md\""
    )


def clean_string(string: str) -> str:
    return (
        string.lower()
        .replace(" ", "_")
        .replace("/", "-")
        .replace("(", "")
        .replace(")", "")
        .replace(":", "")
    )


def read_integrations_js(path_to_file: str):
    """
    Parse integrations/integrations.js and return (categories, integrations).
    """
    try:
        data = Path(path_to_file).read_text()
        categories_str = data.split("export const categories = ")[1].split("export const integrations = ")[0]
        integrations_str = data.split("export const categories = ")[1].split("export const integrations = ")[1]
        return json.loads(categories_str), json.loads(integrations_str)
    except FileNotFoundError as e:
        print("Exception", e)
        return [], []


def generate_category_from_name(category_fragment, category_array) -> str:
    """
    Given a split category id (by ".") and categories tree, return Learn path.
    """
    category_name = ""
    i = 0
    dummy_id = category_fragment[0]

    while i < len(category_fragment):
        for category in category_array:
            if dummy_id == category["id"]:
                category_name += f"/{category['name']}"
                try:
                    dummy_id = f"{dummy_id}.{category_fragment[i + 1]}"
                except IndexError:
                    return category_name.split("/", 1)[1]
                category_array = category["children"]
                break
        i += 1
    return category_name.split("/", 1)[1] if category_name else ""


def create_overview(integration, filename: str, overview_key_name: str = "overview") -> str:
    # Empty overview_key_name => only image on overview
    if not overview_key_name:
        return f"# {integration['meta']['name']}\n\n<img src=\"https://netdata.cloud/img/{filename}\" width=\"150\"/>\n"

    split = re.split(r"(#.*\n)", integration[overview_key_name], maxsplit=1)
    first_overview_part = split[1]
    rest_overview_part = split[2]

    if not filename:
        return f"{first_overview_part}{rest_overview_part}"

    return f"""{first_overview_part}

<img src="https://netdata.cloud/img/{filename}" width="150"/>

{rest_overview_part}"""


def build_readme_from_integration(integration, categories, mode: str = ""):
    """
    Build the README markdown string for an integration.
    Returns (meta_yaml, sidebar_label, learn_rel_path, md, community_badge)
    """
    md = ""
    meta_yaml = ""
    sidebar_label = ""
    learn_rel_path = ""

    try:
        if mode == "collector":
            meta_yaml = integration["edit_link"].replace("blob", "edit")
            sidebar_label = integration["meta"]["monitored_instance"]["name"]
            learn_rel_path = generate_category_from_name(
                integration["meta"]["monitored_instance"]["categories"][0].split("."), categories
            ).replace("Data Collection", "Collecting Metrics")
            keywords = integration["meta"]["keywords"] if "keywords" in integration["meta"] else None

            md = f"""<!--startmeta
meta_yaml: "{meta_yaml}"
sidebar_label: "{sidebar_label}"
learn_status: "Published"
learn_rel_path: "{learn_rel_path}"
"""
            if keywords:
                md += f"keywords: {keywords}\n"

            md+=f"""message: "DO NOT EDIT THIS FILE DIRECTLY, IT IS GENERATED BY THE COLLECTOR'S metadata.yaml FILE"
endmeta-->

{create_overview(integration, integration['meta']['monitored_instance']['icon_filename'])}"""

            if integration.get("metrics"):
                md += f"\n{integration['metrics']}\n"
            if integration.get("functions_index"):
                md += f"\n{integration['functions_index']}\n"
            elif integration.get("functions"):
                md += f"\n{integration['functions']}\n"
            if integration.get("alerts"):
                md += f"\n{integration['alerts']}\n"
            if integration.get("setup"):
                md += f"\n{integration['setup']}\n"
            if integration.get("troubleshooting"):
                md += f"\n{integration['troubleshooting']}\n"

        elif mode == "exporter":
            meta_yaml = integration["edit_link"].replace("blob", "edit")
            sidebar_label = integration["meta"]["name"]
            learn_rel_path = generate_category_from_name(
                integration["meta"]["categories"][0].split("."), categories
            )
            keywords = integration["keywords"] if "keywords" in integration else None

            md = f"""<!--startmeta
meta_yaml: "{meta_yaml}"
sidebar_label: "{sidebar_label}"
learn_status: "Published"
learn_rel_path: "Exporting Metrics/Connectors"
"""
            if keywords:
                md += f"keywords: {keywords}\n"

            md+=f"""message: "DO NOT EDIT THIS FILE DIRECTLY, IT IS GENERATED BY THE EXPORTER'S metadata.yaml FILE"
endmeta-->

{create_overview(integration, integration['meta']['icon_filename'])}"""

            if integration.get("setup"):
                md += f"\n{integration['setup']}\n"
            if integration.get("troubleshooting"):
                md += f"\n{integration['troubleshooting']}\n"

        elif mode == "agent-notification":
            meta_yaml = integration["edit_link"].replace("blob", "edit")
            sidebar_label = integration["meta"]["name"]
            learn_rel_path = generate_category_from_name(
                integration["meta"]["categories"][0].split("."), categories
            )
            keywords = integration["keywords"] if "keywords" in integration else None

            md = f"""<!--startmeta
meta_yaml: "{meta_yaml}"
sidebar_label: "{sidebar_label}"
learn_status: "Published"
learn_rel_path: "{learn_rel_path.replace("notifications", "Alerts & Notifications/Notifications")}"
"""
            if keywords:
                md += f"keywords: {keywords}\n"

            md+=f"""message: "DO NOT EDIT THIS FILE DIRECTLY, IT IS GENERATED BY THE NOTIFICATION'S metadata.yaml FILE"
endmeta-->

{create_overview(integration, integration['meta']['icon_filename'], "overview")}"""

            if integration.get("setup"):
                md += f"\n{integration['setup']}\n"
            if integration.get("troubleshooting"):
                md += f"\n{integration['troubleshooting']}\n"

        elif mode == "cloud-notification":
            meta_yaml = integration["edit_link"].replace("blob", "edit")
            sidebar_label = integration["meta"]["name"]
            learn_rel_path = generate_category_from_name(
                integration["meta"]["categories"][0].split("."), categories
            )
            keywords = integration["keywords"] if "keywords" in integration else None

            md = f"""<!--startmeta
meta_yaml: "{meta_yaml}"
sidebar_label: "{sidebar_label}"
learn_status: "Published"
learn_rel_path: "{learn_rel_path.replace("notifications", "Alerts & Notifications/Notifications")}"
"""
            if keywords:
                md += f"keywords: {keywords}\n"

            md+=f"""message: "DO NOT EDIT THIS FILE DIRECTLY, IT IS GENERATED BY THE NOTIFICATION'S metadata.yaml FILE"
endmeta-->

{create_overview(integration, integration['meta']['icon_filename'], "")}"""

            if integration.get("setup"):
                md += f"\n{integration['setup']}\n"
            if integration.get("troubleshooting"):
                md += f"\n{integration['troubleshooting']}\n"

        elif mode == "logs":
            meta_yaml = integration["edit_link"].replace("blob", "edit")
            sidebar_label = integration["meta"]["name"]
            learn_rel_path = generate_category_from_name(
                integration["meta"]["categories"][0].split("."), categories
            )
            keywords = integration["keywords"] if "keywords" in integration else None

            md = f"""<!--startmeta
meta_yaml: "{meta_yaml}"
sidebar_label: "{sidebar_label}"
learn_status: "Published"
learn_rel_path: "{learn_rel_path.replace("logs", "Logs")}"
"""
            if keywords:
                md += f"keywords: {keywords}\n"

            md+=f"""message: "DO NOT EDIT THIS FILE DIRECTLY, IT IS GENERATED BY THE LOGS' metadata.yaml FILE"
endmeta-->

{create_overview(integration, integration['meta']['icon_filename'])}"""

            if integration.get("setup"):
                md += f"\n{integration['setup']}\n"

        elif mode == "authentication":
            meta_yaml = integration["edit_link"].replace("blob", "edit")
            sidebar_label = integration["meta"]["name"]
            learn_rel_path = generate_category_from_name(
                integration["meta"]["categories"][0].split("."), categories
            )
            keywords = integration["keywords"] if "keywords" in integration else None

            md = f"""<!--startmeta
meta_yaml: "{meta_yaml}"
sidebar_label: "{sidebar_label}"
learn_status: "Published"
learn_rel_path: "{learn_rel_path.replace("authentication", "Netdata Cloud/Authentication & Authorization/Cloud Authentication & Authorization Integrations")}"
"""
            if keywords:
                md += f"keywords: {keywords}\n"

            md+=f"""message: "DO NOT EDIT THIS FILE DIRECTLY, IT IS GENERATED BY THE AUTHENTICATION'S metadata.yaml FILE"
endmeta-->

{create_overview(integration, integration['meta']['icon_filename'])}"""

            if integration.get("setup"):
                md += f"\n{integration['setup']}\n"
            if integration.get("troubleshooting"):
                md += f"\n{integration['troubleshooting']}\n"

    except Exception as e:
        print("Exception building md", e, integration.get("id"))

    # Community badge
    community = '<img src="https://img.shields.io/badge/maintained%20by-Netdata-%2300ab44" />'
    if "community" in integration["meta"]:
        community = '<img src="https://img.shields.io/badge/maintained%20by-Community-blue" />'

    return meta_yaml, sidebar_label, learn_rel_path, md, community


def create_overview_banner(md: str, community_badge: str) -> str:
    """
    Insert the community badge right before the first '##' section.
    """
    if "##" not in md:
        return f"{md}\n\n{community_badge}\n"
    upper, lower = md.split("##", 1)
    return f"{upper}{community_badge}\n\n##{lower}"


def write_to_file(path: str, md: str, meta_yaml: str, sidebar_label: str, community: str, integration=None,
                  mode: str = "default", integration_id: str = None):
    """
    Write the generated markdown into an `integrations/` subdirectory located alongside the `metadata.yaml` file.
    This mirrors the original behavior of placing docs next to their source metadata.
    Also registers the actual output path in id_to_path for later link resolution.
    """
    md = create_overview_banner(md, community)

    if mode == "default":
        base = Path(path)
        if base.exists():
            integrations_dir = base / "integrations"
            integrations_dir.mkdir(exist_ok=True)

            try:
                md2 = add_custom_edit_url(md, meta_yaml, sidebar_label)
                outfile = integrations_dir / f"{clean_string(sidebar_label)}.md"
                clean_and_write(md2, outfile)
                if integration_id:
                    id_to_path[integration_id] = str(outfile)
            except FileNotFoundError as e:
                print("Exception in writing to file", e)

            # If there's only one root-level markdown file, register it for README symlink.
            md_files = [p for p in integrations_dir.iterdir() if p.is_file() and p.suffix == ".md"]
            if len(md_files) == 1:
                symlink_dict.update({path: f"integrations/{clean_string(sidebar_label)}.md"})
            else:
                try:
                    symlink_dict.pop(path)
                except KeyError:
                    pass

    elif mode == "cloud-notification":
        name = clean_string(integration["meta"]["name"])
        base = Path(path)
        integrations_dir = base / "integrations"
        integrations_dir.mkdir(exist_ok=True)
        md2 = add_custom_edit_url(md, meta_yaml, sidebar_label, mode="cloud-notification")
        finalpath = integrations_dir / f"{name}.md"
        try:
            clean_and_write(md2, finalpath)
            if integration_id:
                id_to_path[integration_id] = str(finalpath)
        except FileNotFoundError as e:
            print("Exception in writing to file", e)

    elif mode == "agent-notification":
        md2 = add_custom_edit_url(md, meta_yaml, sidebar_label, mode="agent-notification")
        finalpath = Path(path) / "README.md"
        try:
            clean_and_write(md2, finalpath)
            if integration_id:
                id_to_path[integration_id] = str(finalpath)
        except FileNotFoundError as e:
            print("Exception in writing to file", e)

    elif mode == "logs":
        name = clean_string(integration["meta"]["name"])
        base = Path(path)
        integrations_dir = base / "integrations"
        integrations_dir.mkdir(exist_ok=True)
        md2 = add_custom_edit_url(md, meta_yaml, sidebar_label, mode="logs")
        finalpath = integrations_dir / f"{name}.md"
        try:
            clean_and_write(md2, finalpath)
            if integration_id:
                id_to_path[integration_id] = str(finalpath)
        except FileNotFoundError as e:
            print("Exception in writing to file", e)

    elif mode == "authentication":
        name = clean_string(integration["meta"]["name"])
        base = Path(path)
        integrations_dir = base / "integrations"
        integrations_dir.mkdir(exist_ok=True)
        md2 = add_custom_edit_url(md, meta_yaml, sidebar_label, mode="cloud-authentication")
        finalpath = integrations_dir / f"{name}.md"
        try:
            clean_and_write(md2, finalpath)
            if integration_id:
                id_to_path[integration_id] = str(finalpath)
        except FileNotFoundError as e:
            print("Exception in writing to file", e)


def make_symlinks(symlinks: dict):
    """
    Create README.md symlinks to the sole file in each /integrations dir.
    """
    for element in symlinks:
        readme = Path(element) / "README.md"
        if not readme.exists():
            readme.touch()
        try:
            readme.unlink()
        except FileNotFoundError:
            pass

        readme.symlink_to(symlinks[element])

        filepath = Path(element) / symlinks[element]
        md = filepath.read_text()
        filepath.write_text(md.replace(f"{element}/{symlinks[element]}", f"{element}/README.md"))


# -----------------------------
# Filtering helpers
# -----------------------------
def _base_paths_for_collector(integrations, collector_key: str):
    """
    Return local base paths (without /integrations) for a single collector key: 'plugin/module'
    """
    if not collector_key:
        return []
    paths = []
    for integ in integrations:
        if integ.get("integration_type") != "collector":
            continue
        meta = integ.get("meta", {})
        plugin = meta.get("plugin_name")
        module = meta.get("module_name")
        if not plugin or not module:
            continue
        key = f"{plugin}/{module}"
        if key == collector_key:
            meta_yaml = integ.get("edit_link", "").replace("blob", "edit")
            base = build_path(meta_yaml)
            paths.append(base)
    return paths


# -----------------------------
# CLI entry
# -----------------------------
def main():
    parser = argparse.ArgumentParser(description="Generate integration docs from metadata.yaml files.")
    parser.add_argument(
        "-c",
        "--collector",
        help="Generate docs only for this collector (plugin/module), e.g. 'go.d/snmp' or 'apps.plugin/groups'",
        default=None,
    )
    args = parser.parse_args()

    categories, integrations = read_integrations_js("integrations/integrations.js")
    seed_id_to_expected_paths(integrations)

    if args.collector:
        # compute targets and CLEAN ONLY those
        only_paths = _base_paths_for_collector(integrations, args.collector)
        if not only_paths:
            print(f"No matching collector found for: {args.collector}")
            sys.exit(0)
        cleanup(only_paths)
    else:
        # full cleanup (legacy behavior)
        cleanup()

    # Generate (pass 1: write all files, record id → actual output path)
    for integration in integrations:
        itype = integration.get("integration_type")
        iid = integration.get("id")

        # If -c is used, process ONLY the matching collector; skip everything else
        if args.collector:
            if itype != "collector":
                continue
            meta = integration.get("meta", {})
            plugin = meta.get("plugin_name")
            module = meta.get("module_name")
            if not plugin or not module or f"{plugin}/{module}" != args.collector:
                continue

        if itype == "collector":
            collector_meta_yaml = integration.get("edit_link", "").replace("blob", "edit")
            collector_path = build_path(collector_meta_yaml)
            integration["functions_index"] = render_functions_index(integration, collector_path)

            meta_yaml, sidebar_label, learn_rel_path, md, community = build_readme_from_integration(
                integration, categories, mode="collector"
            )
            write_to_file(collector_path, md, meta_yaml, sidebar_label, community, integration_id=iid)
            write_function_tiles(collector_path, integration, meta_yaml, learn_rel_path)

        elif itype == "exporter" and not args.collector:
            meta_yaml, sidebar_label, learn_rel_path, md, community = build_readme_from_integration(
                integration, categories, mode="exporter"
            )
            path = build_path(meta_yaml)
            write_to_file(path, md, meta_yaml, sidebar_label, community, integration_id=iid)

        elif itype == "agent_notification" and not args.collector:
            meta_yaml, sidebar_label, learn_rel_path, md, community = build_readme_from_integration(
                integration, categories, mode="agent-notification"
            )
            path = build_path(meta_yaml)
            write_to_file(path, md, meta_yaml, sidebar_label, community, integration=integration,
                          mode="agent-notification", integration_id=iid)

        elif itype == "cloud_notification" and not args.collector:
            meta_yaml, sidebar_label, learn_rel_path, md, community = build_readme_from_integration(
                integration, categories, mode="cloud-notification"
            )
            path = build_path(meta_yaml)
            write_to_file(path, md, meta_yaml, sidebar_label, community, integration=integration,
                          mode="cloud-notification", integration_id=iid)

        elif itype == "logs" and not args.collector:
            meta_yaml, sidebar_label, learn_rel_path, md, community = build_readme_from_integration(
                integration, categories, mode="logs"
            )
            path = build_path(meta_yaml)
            write_to_file(path, md, meta_yaml, sidebar_label, community, integration=integration,
                          mode="logs", integration_id=iid)

        elif itype == "authentication" and not args.collector:
            meta_yaml, sidebar_label, learn_rel_path, md, community = build_readme_from_integration(
                integration, categories, mode="authentication"
            )
            path = build_path(meta_yaml)
            write_to_file(path, md, meta_yaml, sidebar_label, community, integration=integration,
                          mode="authentication", integration_id=iid)

    # Pass 2: resolve relatedResource tags to markdown links now that all paths are known
    resolve_related_links()

    make_symlinks(symlink_dict)


if __name__ == "__main__":
    main()
