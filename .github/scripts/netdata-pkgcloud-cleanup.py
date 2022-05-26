#!/bin/env python3

import requests
from requests.auth import HTTPBasicAuth
from datetime import date, datetime, timedelta
import os
import sys
import argparse
from pprint import pprint
from datetime import datetime
from dateutil import parser


class PackageCloud:
    NUM_PACKAGE_MINOR_TO_KEEP = 5
    NUM_RETENTION_DAYS = 30
    # number of pages to process. Use '0' to process all
    MAX_PAGES = 0

    def __init__(self, repo_type, dry_run=True, auth_token=None):
        self.headers = {
            "Accept" : "application/json",
            "Content-Type" : "application/json",
        }
        self.dry_run = dry_run
        self.repo_type = repo_type
        if repo_type == "stable":
            repo = "netdata/netdata"
        elif repo_type == "devel":
            repo = "netdata/netdata-devel"
        elif repo_type == "edge":
            repo = "netdata/netdata-edge"
        else:
            print(f"ERROR: unknown repo type '{repo_type}'!\nAccepted values are: stable,devel,edge")
            sys.exit(1)
        self.base_url = f"https://packagecloud.io/api/v1/repos/{repo}"
        self.auth = HTTPBasicAuth(username=auth_token, password='') if auth_token else None

    def get_all_packages(self):
        page = 1
        all_pkg_list = []
        while True:
            url = f"{self.base_url}/packages.json?page={page}"
            if page > self.MAX_PAGES and self.MAX_PAGES != 0:
                break
            else:
                pkg_list = requests.get(url, auth=self.auth, headers=self.headers).json()
                if len(pkg_list) == 0:
                    break
                else:
                    print(f"Processing page: {page}")
                    for element in pkg_list:
                        self.is_pkg_older_than_days(element, 30)
                        if element['name'] != 'netdata-repo' and element['name'] != 'netdata-repo-edge':
                            all_pkg_list.append(element)
            page += 1
        return all_pkg_list

    def delete_package(self, destroy_url):
        if self.dry_run:
            print(f"         - DRY_RUN mode. Not deleting package '{destroy_url}'.")
        else:
            print(f"         - Deleting package: {destroy_url}")
            url = f"https://packagecloud.io{destroy_url}"
            response = requests.delete(url, auth=self.auth, headers=self.headers).json()
            response = None
            if not response:
                print(f"           Package deleted successfully.")
            else:
                print(f"           Failed deleting package!")

    def get_destroy_url(self, pkg_url):
        url = f"https://packagecloud.io{pkg_url}"
        response = requests.get(url, auth=self.auth, headers=self.headers)
        response.raise_for_status()
        return response.json()['destroy_url']

    def get_packages_for_distro(self, distro, all_pkg_list):
        distro_pkg_list = [ pkg for pkg in all_pkg_list if pkg['distro_version'] == distro ]
        return distro_pkg_list

    def get_packages_for_arch(self, arch, all_pkg_list):
        arch_pkg_list = [ pkg for pkg in all_pkg_list if pkg['package_url'].split('/')[11] == arch ]
        return arch_pkg_list

    def get_arches(self, pkg_list):
        arches = list(set([pkg['package_url'].split('/')[11] for pkg in pkg_list ]))
        return arches

    def get_pkg_list(self, pkg_name, pkg_list):
        filtered_list = [ pkg for pkg in pkg_list if pkg['name'] == pkg_name ]
        return filtered_list

    def get_minor_versions(self, all_versions):
        minor_versions = ['.'.join(version.split('.')[:-1]) for version in all_versions ]
        minor_versions = list(set(minor_versions))
        minor_versions.sort()
        return minor_versions

    def is_pkg_older_than_days(self, pkg, num_days):
        pkg_create_date = datetime.strptime(pkg['created_at'], '%Y-%m-%dT%H:%M:%S.%fZ')
        time_difference = datetime.now() - pkg_create_date
        return time_difference.days > num_days

    def cleanup_repo(self):
        if self.repo_type == 'stable':
            self.cleanup_stable_repo()
        else:
            self.cleanup_edge_repo()

    def cleanup_edge_repo(self):
        all_pkg_list = self.get_all_packages()
        pkgs_to_delete = []
        pkgs_to_keep = []
        for package in all_pkg_list:
            if self.is_pkg_older_than_days(package, self.NUM_RETENTION_DAYS):
                pkgs_to_delete.append(package)
            else:
                pkgs_to_keep.append(package)
        print(f"Keeping the following packages (newer than {self.NUM_RETENTION_DAYS} days):")
        for pkg in pkgs_to_keep:
            print(f"    > pkg: {pkg['package_html_url']} / created_at: {pkg['created_at']}")
        print(f"Deleting the following packages (older than {self.NUM_RETENTION_DAYS} days):")
        for pkg in pkgs_to_delete:
            print(f"    > pkg: {pkg['package_html_url']} / created_at: {pkg['created_at']}")
            self.delete_package(pkg['destroy_url'])

    def cleanup_stable_repo(self):
        all_pkg_list = self.get_all_packages()
        all_distros = list(set([ pkg['distro_version'] for pkg in all_pkg_list ]))
        all_distros = sorted(all_distros)
        print(f"<> Distributions list: {all_distros}")

        for distro in all_distros:
            print(f">> Processing distro: {distro}")
            pkg_list_distro = self.get_packages_for_distro(distro, all_pkg_list)
            arches = self.get_arches(pkg_list_distro)
            print(f"   <> Arch list: {arches}")
            for arch in arches:
                print(f"   >> Processing arch: {distro} -> {arch}")
                pkg_list_arch = self.get_packages_for_arch(arch, pkg_list_distro)
                pkg_names = [pkg['name'] for pkg in pkg_list_arch]
                pkg_names = list(set(pkg_names))
                print(f"     <> Package names: {pkg_names}")
                for pkg_name in pkg_names:
                    print(f"     >> Processing package: {distro} -> {arch} -> {pkg_name}")
                    pkg_list = self.get_pkg_list(pkg_name, pkg_list_arch)
                    pkg_versions = [pkg['version'] for pkg in pkg_list]
                    pkg_minor_versions = self.get_minor_versions(pkg_versions)
                    pkg_minor_to_keep = pkg_minor_versions[-self.NUM_PACKAGE_MINOR_TO_KEEP:]
                    print(f"       <> Minor Package Versions to Keep: {pkg_minor_to_keep}")
                    pkg_minor_to_delete = list(set(pkg_minor_versions) - set(pkg_minor_to_keep))
                    print(f"       <> Minor Package Versions to Delete: {pkg_minor_to_delete}")
                    urls_to_keep = [pkg['package_url'] for pkg in pkg_list if '.'.join(pkg['version'].split('.')[:-1]) in pkg_minor_to_keep]
                    urls_to_delete = [pkg['package_url'] for pkg in pkg_list if '.'.join(pkg['version'].split('.')[:-1]) in pkg_minor_to_delete]
                    for pkg_url in urls_to_delete:
                        destroy_url = self.get_destroy_url(pkg_url)
                        self.delete_package(destroy_url)


def configure():
    parser = argparse.ArgumentParser()
    parser.add_argument('--repo-type', '-r', required=True,
                        help='Repository type against to perform cleanup')
    parser.add_argument('--dry-run', '-d', action='store_true',
                        help='Dry-run Mode')
    args = parser.parse_args()
    try:
        token = os.environ['PKGCLOUD_TOKEN']
    except Exception as e:
        print(f"FATAL: 'PKGCLOUD_TOKEN' environment variable is not set!", file=sys.stderr)
        sys.exit(1)
    repo_type = args.repo_type
    dry_run = args.dry_run
    conf = {
        'repo_type': args.repo_type,
        'dry_run': args.dry_run,
        'token': token
    }
    return conf


def main():
    config = configure()
    pkg_cloud = PackageCloud(config['repo_type'], config['dry_run'], config['token'])
    pkg_cloud.cleanup_repo()


if __name__ == "__main__":
    main()
