#!/usr/bin/env python3
import argparse
import json
import re
import shutil
import sys
from pathlib import Path

# Registry used to decide which README.md should symlink to which generated file
symlink_dict = {}


# -----------------------------
# FS utilities
# -----------------------------
def cleanup(only_base_paths=None):
    """
    Clean generated /integrations folders.
    - If only_base_paths is provided (list of base dirs), clean ONLY those.
    - Otherwise, do a full cleanup (legacy behavior).
    """
    if only_base_paths:
        for base in only_base_paths:
            p = Path(base) / "integrations"
            if p.exists():
                shutil.rmtree(p)
        return

    for element in Path("src/go/plugin/go.d/collector").glob("**/*/"):
        if "integrations" in str(element):
            shutil.rmtree(element)
    for element in Path("src/collectors").glob("**/*/"):
        if "integrations" in str(element):
            shutil.rmtree(element)
    for element in Path("src/exporting").glob("**/*/"):
        if "integrations" in str(element):
            shutil.rmtree(element)
    for element in Path("integrations/cloud-notifications").glob("**/*/"):
        if "integrations" in str(element) and "metadata.yaml" not in str(element):
            shutil.rmtree(element)
    for element in Path("integrations/logs").glob("**/*/"):
        if "integrations" in str(element) and "metadata.yaml" not in str(element):
            shutil.rmtree(element)
    for element in Path("integrations/cloud-authentication").glob("**/*/"):
        if "integrations" in str(element) and "metadata.yaml" not in str(element):
            shutil.rmtree(element)


def clean_and_write(md: str, path: Path):
    """
    Convert custom {% details %} markers to HTML <details> and write file.
    """
    md = md.replace('{% details summary="', "<details><summary>")
    md = md.replace('{% details open=true summary="', "<details open><summary>")
    md = md.replace('" %}', "</summary>\n")
    md = md.replace("{% /details %}", "</details>\n")
    path.write_text(md)


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

    split = re.split(r"(#.*\n)", integration[overview_key_name], 1)
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
            most_popular = integration["meta"]["most_popular"]

            md = f"""<!--startmeta
meta_yaml: "{meta_yaml}"
sidebar_label: "{sidebar_label}"
learn_status: "Published"
learn_rel_path: "{learn_rel_path}"
most_popular: {most_popular}
message: "DO NOT EDIT THIS FILE DIRECTLY, IT IS GENERATED BY THE COLLECTOR'S metadata.yaml FILE"
endmeta-->

{create_overview(integration, integration['meta']['monitored_instance']['icon_filename'])}"""

            if integration.get("metrics"):
                md += f"\n{integration['metrics']}\n"
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

            md = f"""<!--startmeta
meta_yaml: "{meta_yaml}"
sidebar_label: "{sidebar_label}"
learn_status: "Published"
learn_rel_path: "Exporting Metrics/Connectors"
message: "DO NOT EDIT THIS FILE DIRECTLY, IT IS GENERATED BY THE EXPORTER'S metadata.yaml FILE"
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

            md = f"""<!--startmeta
meta_yaml: "{meta_yaml}"
sidebar_label: "{sidebar_label}"
learn_status: "Published"
learn_rel_path: "{learn_rel_path.replace("notifications", "Alerts & Notifications/Notifications")}"
message: "DO NOT EDIT THIS FILE DIRECTLY, IT IS GENERATED BY THE NOTIFICATION'S metadata.yaml FILE"
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

            md = f"""<!--startmeta
meta_yaml: "{meta_yaml}"
sidebar_label: "{sidebar_label}"
learn_status: "Published"
learn_rel_path: "{learn_rel_path.replace("notifications", "Alerts & Notifications/Notifications")}"
message: "DO NOT EDIT THIS FILE DIRECTLY, IT IS GENERATED BY THE NOTIFICATION'S metadata.yaml FILE"
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

            md = f"""<!--startmeta
meta_yaml: "{meta_yaml}"
sidebar_label: "{sidebar_label}"
learn_status: "Published"
learn_rel_path: "{learn_rel_path.replace("logs", "Logs")}"
message: "DO NOT EDIT THIS FILE DIRECTLY, IT IS GENERATED BY THE LOGS' metadata.yaml FILE"
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

            md = f"""<!--startmeta
meta_yaml: "{meta_yaml}"
sidebar_label: "{sidebar_label}"
learn_status: "Published"
learn_rel_path: "{learn_rel_path.replace("authentication", "Netdata Cloud/Authentication & Authorization/Cloud Authentication & Authorization Integrations")}"
message: "DO NOT EDIT THIS FILE DIRECTLY, IT IS GENERATED BY THE AUTHENTICATION'S metadata.yaml FILE"
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
    upper, lower = md.split("##", 1)
    return f"{upper}{community_badge}\n\n##{lower}"


def write_to_file(path: str, md: str, meta_yaml: str, sidebar_label: str, community: str, integration=None,
                  mode: str = "default"):
    """
    Write the generated markdown into an `integrations/` subdirectory located alongside the `metadata.yaml` file.
    This mirrors the original behavior of placing docs next to their source metadata.
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
            except FileNotFoundError as e:
                print("Exception in writing to file", e)

            # If there's only one file inside the directory, register it for README symlink
            if len(list(integrations_dir.iterdir())) == 1:
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
        except FileNotFoundError as e:
            print("Exception in writing to file", e)

    elif mode == "agent-notification":
        md2 = add_custom_edit_url(md, meta_yaml, sidebar_label, mode="agent-notification")
        finalpath = Path(path) / "README.md"
        try:
            clean_and_write(md2, finalpath)
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

    # Generate
    for integration in integrations:
        itype = integration.get("integration_type")

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
            meta_yaml, sidebar_label, learn_rel_path, md, community = build_readme_from_integration(
                integration, categories, mode="collector"
            )
            path = build_path(meta_yaml)
            write_to_file(path, md, meta_yaml, sidebar_label, community)

        elif itype == "exporter" and not args.collector:
            meta_yaml, sidebar_label, learn_rel_path, md, community = build_readme_from_integration(
                integration, categories, mode="exporter"
            )
            path = build_path(meta_yaml)
            write_to_file(path, md, meta_yaml, sidebar_label, community)

        elif itype == "agent_notification" and not args.collector:
            meta_yaml, sidebar_label, learn_rel_path, md, community = build_readme_from_integration(
                integration, categories, mode="agent-notification"
            )
            path = build_path(meta_yaml)
            write_to_file(path, md, meta_yaml, sidebar_label, community, integration=integration,
                          mode="agent-notification")

        elif itype == "cloud_notification" and not args.collector:
            meta_yaml, sidebar_label, learn_rel_path, md, community = build_readme_from_integration(
                integration, categories, mode="cloud-notification"
            )
            path = build_path(meta_yaml)
            write_to_file(path, md, meta_yaml, sidebar_label, community, integration=integration,
                          mode="cloud-notification")

        elif itype == "logs" and not args.collector:
            meta_yaml, sidebar_label, learn_rel_path, md, community = build_readme_from_integration(
                integration, categories, mode="logs"
            )
            path = build_path(meta_yaml)
            write_to_file(path, md, meta_yaml, sidebar_label, community, integration=integration, mode="logs")

        elif itype == "authentication" and not args.collector:
            meta_yaml, sidebar_label, learn_rel_path, md, community = build_readme_from_integration(
                integration, categories, mode="authentication"
            )
            path = build_path(meta_yaml)
            write_to_file(path, md, meta_yaml, sidebar_label, community, integration=integration, mode="authentication")

    make_symlinks(symlink_dict)


if __name__ == "__main__":
    main()
