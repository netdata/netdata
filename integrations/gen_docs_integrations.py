import json
import shutil
from pathlib import Path
import re

# Dictionary responsible for making the symbolic links at the end of the script's run.
symlink_dict = {}
am_i_inside_go = "go.d.plugin" in str(Path.cwd())


def cleanup():
    """
    clean directories that are either data collection or exporting integrations
    """
    if am_i_inside_go:
        for element in Path("modules").glob('**/*/'):
            if "integrations" in str(element):
                shutil.rmtree(element)
    else:
        for element in Path("collectors").glob('**/*/'):
            if "integrations" in str(element):
                shutil.rmtree(element)

        for element in Path("exporting").glob('**/*/'):
            if "integrations" in str(element):
                shutil.rmtree(element)
        for element in Path("integrations/cloud-notifications").glob('**/*/'):
            if "integrations" in str(element) and "metadata.yaml" not in str(
                element
            ):
                shutil.rmtree(element)


def generate_category_from_name(category_fragment, category_array):
    """
    Takes a category ID in splitted form ("." as delimiter) and the array of the categories, and returns the proper category name that Learn expects.
    """

    category_name = ""
    i = 0
    dummy_id = category_fragment[0]

    while i < len(category_fragment):
        for category in category_array:

            if dummy_id == category['id']:
                category_name = f"{category_name}/" + category["name"]
                try:
                    # print("equals")
                    # print(fragment, category_fragment[i+1])
                    dummy_id = f"{dummy_id}.{category_fragment[i + 1]}"
                                    # print(dummy_id)
                except IndexError:
                    return category_name.split("/", 1)[1]
                category_array = category['children']
                break
        i += 1


def clean_and_write(md, path):
    """
    This function takes care of the special details element, and converts it to the equivalent that md expects.
    Then it writes the buffer on the file provided.
    """
    # clean first, replace
    md = md.replace("{% details summary=\"", "<details><summary>").replace(
        "\" %}", "</summary>\n").replace("{% /details %}", "</details>\n")

    path.write_text(md)


def add_custom_edit_url(markdown_string, meta_yaml_link, sidebar_label_string, mode='default'):
    """
    Takes a markdown string and adds a "custom_edit_url" metadata to the metadata field
    """

    output = ""

    if mode == 'default':
        path_to_md_file = f'{meta_yaml_link.replace("/metadata.yaml", "")}/integrations/{clean_string(sidebar_label_string)}'

    elif mode == 'cloud-notifications':
        path_to_md_file = meta_yaml_link.replace("metadata.yaml", f'integrations/{clean_string(sidebar_label_string)}')

    elif mode == 'agent-notifications':
        path_to_md_file = meta_yaml_link.replace("metadata.yaml", "README")

    return markdown_string.replace(
        "<!--startmeta",
        f'<!--startmeta\ncustom_edit_url: \"{path_to_md_file}.md\"',
    )


def clean_string(string):
    """
    simple function to get rid of caps, spaces, slashes and parentheses from a given string

    The string represents an integration name, as it would be displayed in the final text
    """

    return string.lower().replace(" ", "_").replace("/", "-").replace("(", "").replace(")", "").replace(":", "")


def read_integrations_js(path_to_file):
    """
    Open integrations/integrations.js and extract the dictionaries
    """

    try:
        data = Path(path_to_file).read_text()

        categories_str = data.split("export const categories = ")[1].split("export const integrations = ")[0]
        integrations_str = data.split("export const categories = ")[1].split("export const integrations = ")[1]

        return json.loads(categories_str), json.loads(integrations_str)

    except FileNotFoundError as e:
        print("Exception", e)


def create_overview(integration, filename):

    split = re.split(r'(#.*\n)', integration['overview'], 1)

    first_overview_part = split[1]
    rest_overview_part = split[2]

    if len(filename) > 0:
        return f"""{first_overview_part}

<img src="https://netdata.cloud/img/{filename}" width="150"/>

{rest_overview_part}
"""
    else:
        return f"""{first_overview_part}{rest_overview_part}
"""


def build_readme_from_integration(integration, mode=''):
    # COLLECTORS
    if mode == 'collector':

        try:
            # initiate the variables for the collector
            meta_yaml = integration['edit_link'].replace("blob", "edit")
            sidebar_label = integration['meta']['monitored_instance']['name']
            learn_rel_path = generate_category_from_name(
                integration['meta']['monitored_instance']['categories'][0].split("."), categories)
            most_popular = integration['meta']['most_popular']

            # build the markdown string
            md = \
                f"""<!--startmeta
meta_yaml: "{meta_yaml}"
sidebar_label: "{sidebar_label}"
learn_status: "Published"
learn_rel_path: "{learn_rel_path}"
most_popular: {most_popular}
message: "DO NOT EDIT THIS FILE DIRECTLY, IT IS GENERATED BY THE COLLECTOR'S metadata.yaml FILE"
endmeta-->

{create_overview(integration, integration['meta']['monitored_instance']['icon_filename'])}"""

            if integration['metrics']:
                md += f"""
{integration['metrics']}
"""

            if integration['alerts']:
                md += f"""
{integration['alerts']}
"""

            if integration['setup']:
                md += f"""
{integration['setup']}
"""

            if integration['troubleshooting']:
                md += f"""
{integration['troubleshooting']}
"""
        except Exception as e:
            print("Exception in collector md construction", e, integration['id'])

    # EXPORTERS
    elif mode == 'exporter':
        try:
            # initiate the variables for the exporter
            meta_yaml = integration['edit_link'].replace("blob", "edit")
            sidebar_label = integration['meta']['name']
            learn_rel_path = generate_category_from_name(integration['meta']['categories'][0].split("."), categories)

            # build the markdown string
            md = \
                f"""<!--startmeta
meta_yaml: "{meta_yaml}"
sidebar_label: "{sidebar_label}"
learn_status: "Published"
learn_rel_path: "Exporting"
message: "DO NOT EDIT THIS FILE DIRECTLY, IT IS GENERATED BY THE EXPORTER'S metadata.yaml FILE"
endmeta-->

{create_overview(integration, integration['meta']['icon_filename'])}"""

            if integration['setup']:
                md += f"""
{integration['setup']}
"""

            if integration['troubleshooting']:
                md += f"""
{integration['troubleshooting']}
"""
        except Exception as e:
            print("Exception in exporter md construction", e, integration['id'])

    # NOTIFICATIONS
    elif mode == 'notification':
        try:
            # initiate the variables for the notification method
            meta_yaml = integration['edit_link'].replace("blob", "edit")
            sidebar_label = integration['meta']['name']
            learn_rel_path = generate_category_from_name(integration['meta']['categories'][0].split("."), categories)

            # build the markdown string
            md = \
                f"""<!--startmeta
meta_yaml: "{meta_yaml}"
sidebar_label: "{sidebar_label}"
learn_status: "Published"
learn_rel_path: "{learn_rel_path.replace("notifications", "Alerting/Notifications")}"
message: "DO NOT EDIT THIS FILE DIRECTLY, IT IS GENERATED BY THE NOTIFICATION'S metadata.yaml FILE"
endmeta-->

{create_overview(integration, integration['meta']['icon_filename'])}"""

            if integration['setup']:
                md += f"""
{integration['setup']}
"""

            if integration['troubleshooting']:
                md += f"""
{integration['troubleshooting']}
"""

        except Exception as e:
            print("Exception in notification md construction", e, integration['id'])

    if "community" in integration['meta'].keys():
        community = "<img src=\"https://img.shields.io/badge/maintained%20by-Community-blue\" />"
    else:
        community = "<img src=\"https://img.shields.io/badge/maintained%20by-Netdata-%2300ab44\" />"

    return meta_yaml, sidebar_label, learn_rel_path, md, community


def build_path(meta_yaml_link):
    """
    function that takes a metadata yaml file link, and makes it into a path that gets used to write to a file.
    """
    return meta_yaml_link.replace("https://github.com/netdata/", "") \
        .split("/", 1)[1] \
        .replace("edit/master/", "") \
        .replace("/metadata.yaml", "")


def write_to_file(path, md, meta_yaml, sidebar_label, community, mode='default'):
    """
    takes the arguments needed to write the integration markdown to the proper file.
    """

    upper, lower = md.split("##", 1)

    md = upper + community + f"\n\n##{lower}"

    if mode == 'default':
        # Only if the path exists, this caters for running the same script on both the go and netdata repos.
        if Path(path).exists():
            if not Path(f'{path}/integrations').exists():
                Path(f'{path}/integrations').mkdir()

            try:
                md = add_custom_edit_url(md, meta_yaml, sidebar_label)
                clean_and_write(
                    md,
                    Path(f'{path}/integrations/{clean_string(sidebar_label)}.md')
                )

            except FileNotFoundError as e:
                print("Exception in writing to file", e)

            # If we only created one file inside the directory, add the entry to the symlink_dict, so we can make the symbolic link
            if len(list(Path(f'{path}/integrations').iterdir())) == 1:
                symlink_dict.update(
                    {path: f'integrations/{clean_string(sidebar_label)}.md'})
            else:
                try:
                    symlink_dict.pop(path)
                except KeyError:
                    # We don't need to print something here.
                    pass
    elif mode == 'notification':

        if "cloud-notifications" in path:
            # for cloud notifications we generate them near their metadata.yaml
            name = clean_string(integration['meta']['name'])

            if not Path(f'{path}/integrations').exists():
                Path(f'{path}/integrations').mkdir()

            # proper_edit_name = meta_yaml.replace(
            #     "metadata.yaml", f'integrations/{clean_string(sidebar_label)}.md\"')

            md = add_custom_edit_url(md, meta_yaml, sidebar_label, mode='cloud-notifications')

            finalpath = f'{path}/integrations/{name}.md'
        else:
            # add custom_edit_url as the md file, so we can have uniqueness in the ingest script
            # afterwards the ingest will replace this metadata with meta_yaml
            md = add_custom_edit_url(md, meta_yaml, sidebar_label, mode='agent-notifications')

            finalpath = f'{path}/README.md'

        try:
            clean_and_write(
                md,
                Path(finalpath)
            )

        except FileNotFoundError as e:
            print("Exception in writing to file", e)


def make_symlinks(symlink_dict):
    """
    takes a dictionary with directories that have a 1:1 relationship between their README and the integration (only one) inside the "integrations" folder.
    """
    for element in symlink_dict:
        # Remove the README to prevent it being a normal file
        Path(f'{element}/README.md').unlink()
        # and then make a symlink to the actual markdown
        Path(f'{element}/README.md').symlink_to(symlink_dict[element])

        filepath = Path(f'{element}/{symlink_dict[element]}')
        md = filepath.read_text()

        # This preserves the custom_edit_url for most files as it was,
        # so the existing links don't break, this is vital for link replacement afterwards
        filepath.write_text(md.replace(
            f'{element}/{symlink_dict[element]}', f'{element}/README.md'))


cleanup()

categories, integrations = read_integrations_js('integrations/integrations.js')


# Iterate through every integration
for integration in integrations:

    if integration['integration_type'] == "collector":

        meta_yaml, sidebar_label, learn_rel_path, md, community = build_readme_from_integration(
            integration, mode='collector')
        path = build_path(meta_yaml)
        write_to_file(path, md, meta_yaml, sidebar_label, community)

    elif not am_i_inside_go:
        # kind of specific if clause, so we can avoid running excessive code in the go repo
        if integration['integration_type'] == "exporter":

            meta_yaml, sidebar_label, learn_rel_path, md, community = build_readme_from_integration(
                integration, mode='exporter')
            path = build_path(meta_yaml)
            write_to_file(path, md, meta_yaml, sidebar_label, community)

        # kind of specific if clause, so we can avoid running excessive code in the go repo
        elif integration['integration_type'] == "notification":

            meta_yaml, sidebar_label, learn_rel_path, md, community = build_readme_from_integration(
                integration, mode='notification')
            path = build_path(meta_yaml)
            write_to_file(path, md, meta_yaml, sidebar_label, community,  mode='notification')


make_symlinks(symlink_dict)
