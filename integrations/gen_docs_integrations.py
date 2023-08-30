import json
import os

path_dict = {}

path_dict["collector"] = "collectors/"
path_dict["exporter"] = "exporting/"
path_dict["notification"] = "health/notifications/"

symlink_dict = {}


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
                category_name = category_name + "/" + category["name"]
                try:
                    # print("equals")
                    # print(fragment, category_fragment[i+1])
                    dummy_id = dummy_id + "." + category_fragment[i+1]
                    # print(dummy_id)
                except:
                    return category_name.split("/", 1)[1]
                category_array = category['children']
                break
        i += 1


def clean_and_write(md, txt):
    # clean first, replace
    md = md.replace("{% details summary=\"", "<details><summary>").replace(
        "\" %}", "</summary>\n").replace("{% /details %}", "</details>\n")
    # print(md)
    # exit()

    txt.write(md)


with open('integrations/integrations.js') as dataFile:
    data = dataFile.read()
    categories_str = data.split("export const categories = ")[1].split("export const integrations = ")[0]
    integrations_str = data.split("export const categories = ")[1].split("export const integrations = ")[1]
    # jsonObj = json.loads("{" + f"{categories_obj}" + "}")
    categories = json.loads(categories_str)

    integrations = json.loads(integrations_str)

# print(integrations[0].keys())

# print(integrations[0]["integration_type"])

# COLLECTORS
for integration in integrations:
    # integration = integrations[0]
    if integration['integration_type'] == "collector":

        try:
            custom_edit_url = integration['edit_link'].replace("blob", "edit")

            sidebar_label = integration['meta']['monitored_instance']['name']

            learn_rel_path = generate_category_from_name(
                integration['meta']['monitored_instance']['categories'][0].split("."), categories)
            md = f"""
---
custom_edit_url: "{custom_edit_url}"
sidebar_label: "{sidebar_label}"
learn_status: "Published"
learn_rel_path "{learn_rel_path}"
---

{integration['overview']}
"""

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

            path = custom_edit_url.replace("https://github.com/netdata/", "").split("/",
                                                                                    1)[1].replace("edit/master/", "").replace("/metadata.yaml", "")
            if os.path.exists(path):
                try:
                    if not os.path.exists(f'{path}/integrations'):

                        os.mkdir(f'{path}/integrations')
                    with open(f'{path}/integrations/{sidebar_label.lower().replace(" ", "_").replace("/", "-")}.md', 'w+') as txt:
                        clean_and_write(md, txt)
                except Exception as e:
                    print("Error in writing to the file", str(e), integration['id'])

                if len(os.listdir(f'{path}/integrations')) == 1:
                    symlink_dict.update(
                        {path: f'integrations/{sidebar_label.lower().replace(" ", "_").replace("/", "-")}.md'})
                else:
                    try:
                        symlink_dict.pop(path)
                    except Exception as e:
                        pass
                        # print("Info: No entry in symlink dict anyway for key ->", e)

        except Exception as e:
            print("Exception in md construction", str(e), integration['id'])

    elif integration['integration_type'] == "exporter":
        try:
            # print(integration.keys())
            custom_edit_url = integration['edit_link'].replace("blob", "edit")

            sidebar_label = integration['meta']['name']

            learn_rel_path = generate_category_from_name(integration['meta']['categories'][0].split("."), categories)

            md = f"""
---
custom_edit_url: "{custom_edit_url}"
sidebar_label: "{sidebar_label}"
learn_status: "Published"
learn_rel_path "{learn_rel_path}"
---

{integration['overview']}
"""

            if integration['setup']:
                md += f"""
{integration['setup']}
"""

            if integration['troubleshooting']:
                md += f"""
{integration['troubleshooting']}
"""

            path = custom_edit_url.replace("https://github.com/netdata/", "").split("/",
                                                                                    1)[1].replace("edit/master/", "").replace("/metadata.yaml", "")

            try:
                with open(f'{path}/README.md', 'w') as txt:
                    clean_and_write(md, txt)
            except:
                pass
        except Exception as e:
            print(str(e), "\n", integration)
            quit()
    elif integration['integration_type'] == "notification":
        try:
            custom_edit_url = integration['edit_link'].replace("blob", "edit")

            sidebar_label = integration['meta']['name']

            learn_rel_path = generate_category_from_name(integration['meta']['categories'][0].split("."), categories)

            md = f"""
---
custom_edit_url: "{custom_edit_url}"
sidebar_label: "{sidebar_label}"
learn_status: "Published"
learn_rel_path "{learn_rel_path}"
---

{integration['overview']}
"""

            if integration['setup']:
                md += f"""
{integration['setup']}
"""

            if integration['troubleshooting']:
                md += f"""
{integration['troubleshooting']}
"""

            path = custom_edit_url.replace("https://github.com/netdata/", "").split("/",
                                                                                    1)[1].replace("edit/master/", "").replace("/metadata.yaml", "")

            if "cloud-notifications" in path:
                name = integration['meta']['name'].lower().replace(" ", "_")
                finalpath = f'{path}/{name}.md'
            else:
                finalpath = f'{path}/README.md'
            try:
                with open(finalpath, 'w') as txt:
                    clean_and_write(md, txt)
            except Exception as e:
                print(str(e), integration['id'])

        except Exception as e:
            print(str(e), "\n", integration)


# print(symlink_dict)

for element in symlink_dict:
    # print(element,symlink_dict[element])
    os.remove(f'{element}/README.md')

    # print(f'{element}/{symlink_dict[element]}',f'{element}/README.md')

    os.symlink(f'{symlink_dict[element]}',f'{element}/README.md')
    # with open(, 'w') as txt:
    #     str = 
    #     print(str)
    #     txt.write(str)
    # quit()

