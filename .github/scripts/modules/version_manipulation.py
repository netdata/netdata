import os
import re
import requests
from itertools import groupby
from github import Github
from github.GithubException import GithubException

repos_URL = {
    "stable": "netdata/netdata",
    "nightly": "netdata/netdata-nightlies"
}

GH_TOKEN = os.getenv("GH_TOKEN")
if GH_TOKEN is None or GH_TOKEN != "":
    print("Token is not defined or empty, continuing with limitation on requests per sec towards Github API")


def identify_channel(_version):
    nightly_pattern = r'v(\d+)\.(\d+)\.(\d+)-(\d+)-nightly'
    stable_pattern = r'v(\d+)\.(\d+)\.(\d+)'
    if re.match(nightly_pattern, _version):
        _channel = "nightly"
        _pattern = nightly_pattern
    elif re.match(stable_pattern, _version):
        _channel = "stable"
        _pattern = stable_pattern
    else:
        print("Invalid version format.")
        return None
    return _channel, _pattern


def padded_version(item):
    key_value = '10000'
    for value in item[1:]:
        key_value += f'{value:05}'
    return int(key_value)


def extract_version(title):
    if identify_channel(title):
        _, _pattern = identify_channel(title)
    try:
        match = re.match(_pattern, title)
        if match:
            return tuple(map(int, match.groups()))
    except Exception as e:
        print(f"Unexpected error: {e}")
        return None


def get_release_path_and_filename(_version):
    nightly_pattern = r'v(\d+)\.(\d+)\.(\d+)-(\d+)-nightly'
    stable_pattern = r'v(\d+)\.(\d+)\.(\d+)'
    if match := re.match(nightly_pattern, _version):
        msb = match.group(1)
        _path = "nightly"
        _filename = f"v{msb}"
    elif match := re.match(stable_pattern, _version):
        msb = match.group(1)
        _path = "stable"
        _filename = f"v{msb}"
    else:
        print("Invalid version format.")
        exit(1)
    return (_path, _filename)


def compare_version_with_remote(version):
    """
    If the version = fun (version) you need to update the version in the
    remote. If the version remote doesn't exist, returns the version
    :param channel: any version of the agent
    :return: the greater from version and version remote.
    """

    prefix = "https://packages.netdata.cloud/releases"
    path, filename = get_release_path_and_filename(version)

    remote_url = f"{prefix}/{path}/{filename}"
    response = requests.get(remote_url)

    if response.status_code == 200:
        version_remote = response.text.rstrip()

        version_components = extract_version(version)
        remote_version_components = extract_version(version_remote)

        absolute_version = padded_version(version_components)
        absolute_remote_version = padded_version(remote_version_components)

        if absolute_version > absolute_remote_version:
            print(f"Version in the remote: {version_remote}, is older than the current: {version}, I need to update")
            return (version)
        else:
            print(f"Version in the remote: {version_remote}, is newer than the current: {version}, no action needed")
            return (None)
    else:
        # Remote version not found
        print(f"Version in the remote not found, updating the predefined latest path with the version: {version}")
        return (version)


def sort_and_grouby_major_agents_of_channel(channel):
    """
    Fetches the GH API and read either netdata/netdata or netdata/netdata-nightlies repo. It fetches all of their
    releases implements a grouping by their major release number.
    Every k,v in this dictionary is in the form; "vX": [descending ordered list of Agents in this major release].
    :param channel: "nightly" or "stable"
    :return: None or dict() with the Agents grouped by major version # (vX)
    """
    try:
        G = Github(GH_TOKEN)
        repo = G.get_repo(repos_URL[channel])
        releases = repo.get_releases()
    except GithubException as e:
        print(f"GitHub API request failed: {e}")
        return None

    except Exception as e:
        print(f"An unexpected error occurred: {e}")
        return None

    extracted_titles = [extract_version(item.title) for item in releases if
                        extract_version(item.title) is not None]
    # Necessary sorting for implement the group by
    extracted_titles.sort(key=lambda x: x[0])
    # Group titles by major version
    grouped_by_major = {major: list(group) for major, group in groupby(extracted_titles, key=lambda x: x[0])}
    sorted_grouped_by_major = {}
    for key, values in grouped_by_major.items():
        sorted_values = sorted(values, key=padded_version, reverse=True)
        sorted_grouped_by_major[key] = sorted_values
    # Transform them in the correct form
    if channel == "stable":
        result_dict = {f"v{key}": [f"v{a}.{b}.{c}" for a, b, c in values] for key, values in
                       sorted_grouped_by_major.items()}
    else:
        result_dict = {f"v{key}": [f"v{a}.{b}.{c}-{d}-nightly" for a, b, c, d in values] for key, values in
                       sorted_grouped_by_major.items()}
    return result_dict
