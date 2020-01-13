#!/usr/bin/env python3
#
#
# Prepare the build environment within the container
# The script attaches to the running container and does the following:
# 1) Create the container
# 2) Start the container up
# 3) Create the builder user
# 4) Prepare the environment for RPM build
#
# Copyright: SPDX-License-Identifier: GPL-3.0-or-later
#
# Author   : Pavlos Emm. Katsoulakis <paul@netdata.cloud>

import common
import os
import sys
import lxc

if len(sys.argv) != 2:
    print('You need to provide a container name to get things started')
    sys.exit(1)
container_name=sys.argv[1]

# Setup the container object
print("Defining container %s" % container_name)
container = lxc.Container(container_name)
if not container.defined:
    raise Exception("Container %s not defined!" % container_name)

# Start the container
if not container.start():
    raise Exception("Failed to start the container")

if not container.running or not container.state == "RUNNING":
    raise Exception('Container %s is not running, configuration process aborted ' % container_name)

# Wait for connectivity
print("Waiting for container connectivity to start configuration sequence")
if not container.get_ips(timeout=30):
    raise Exception("Timeout while waiting for container")

# Run the required activities now
# Create the builder user
print("1. Adding user %s" % os.environ['BUILDER_NAME'])
common.run_command(container, ["useradd", "-m", os.environ['BUILDER_NAME']])

# Fetch package dependencies for the build
print("2.1 Preparing repo on LXC container")
common.prepare_repo(container)

common.run_command(container, ["wget", "-T", "15", "-O", "/home/%s/.install-required-packages.sh" % (os.environ['BUILDER_NAME']), "https://raw.githubusercontent.com/netdata/netdata/master/packaging/installer/install-required-packages.sh"])
common.run_command(container, ["bash", "/home/%s/.install-required-packages.sh" % (os.environ['BUILDER_NAME']), "netdata", "--dont-wait", "--non-interactive"])

# Exceptional cases, not available everywhere
#
print("2.2 Running uncommon dependencies and preparing LXC environment")
# Not on Centos-7
if os.environ["BUILD_STRING"].count("el/7") <= 0:
    common.run_command(container, [os.environ["REPO_TOOL"], "install", "-y", "libnetfilter_acct-devel"])

# Not on Centos-6
if os.environ["BUILD_STRING"].count("el/6") <= 0:
    common.run_command(container, [os.environ["REPO_TOOL"], "install", "-y", "autoconf-archive"])

print("2.3 Installing common dependencies")
common.install_common_dependendencies(container)

print("3. Setting up macros")
common.run_command(container, ["sudo", "-u", os.environ['BUILDER_NAME'], "/bin/echo", "'%_topdir %(echo /home/" + os.environ['BUILDER_NAME'] + ")/rpmbuild' > /home/" + os.environ['BUILDER_NAME'] + "/.rpmmacros"])

print("4. Create rpmbuild directory")
common.run_command(container, ["sudo", "-u", os.environ['BUILDER_NAME'], "mkdir", "-p", "/home/" + os.environ['BUILDER_NAME'] + "/rpmbuild/BUILD"])
common.run_command(container, ["sudo", "-u", os.environ['BUILDER_NAME'], "mkdir", "-p", "/home/" + os.environ['BUILDER_NAME'] + "/rpmbuild/RPMS"])
common.run_command(container, ["sudo", "-u", os.environ['BUILDER_NAME'], "mkdir", "-p", "/home/" + os.environ['BUILDER_NAME'] + "/rpmbuild/SOURCES"])
common.run_command(container, ["sudo", "-u", os.environ['BUILDER_NAME'], "mkdir", "-p", "/home/" + os.environ['BUILDER_NAME'] + "/rpmbuild/SPECS"])
common.run_command(container, ["sudo", "-u", os.environ['BUILDER_NAME'], "mkdir", "-p", "/home/" + os.environ['BUILDER_NAME'] + "/rpmbuild/SRPMS"])
common.run_command(container, ["sudo", "-u", os.environ['BUILDER_NAME'], "ls", "-ltrR", "/home/" + os.environ['BUILDER_NAME'] + "/rpmbuild"])

# Download the source
rpm_friendly_version=""
dest_archive=""
download_url=""
spec_file="/home/%s/rpmbuild/SPECS/netdata.spec" % os.environ['BUILDER_NAME']
tag = None
rpm_friendly_version, tag = common.fetch_version(os.environ['BUILD_VERSION'])
tar_file="%s/netdata-%s.tar.gz" % (os.path.dirname(dest_archive), rpm_friendly_version)

print("5. I will be building version '%s' of netdata." % os.environ['BUILD_VERSION'])
dest_archive="/home/%s/rpmbuild/SOURCES/netdata-%s.tar.gz" % (os.environ['BUILDER_NAME'], rpm_friendly_version)

common.prepare_version_source(dest_archive, rpm_friendly_version, tag=tag)

# Extract the spec file in place
print("6. Extract spec file from the source")
common.run_command_in_host(['sudo', 'cp', 'netdata.spec', os.environ['LXC_CONTAINER_ROOT'] + spec_file])
common.run_command_in_host(['sudo', 'chmod', '777', os.environ['LXC_CONTAINER_ROOT'] + spec_file])

print("7. Temporary hack: Change Source0 to %s on spec file %s" % (dest_archive, spec_file))
common.replace_tag("Source0", os.environ['LXC_CONTAINER_ROOT'] + spec_file, tar_file)

print('Done!')
