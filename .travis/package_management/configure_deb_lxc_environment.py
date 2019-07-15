#!/usr/bin/env python3
#
# Prepare the build environment within the container
# The script attaches to the running container and does the following:
# 1) Create the container
# 2) Start the container up
# 3) Create the builder user
# 4) Prepare the environment for DEB build
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

build_path = "/home/%s" % os.environ['BUILDER_NAME']

# Run the required activities now
# 1. Create the builder user
print("1. Adding user %s" % os.environ['BUILDER_NAME'])
common.run_command(container, ["useradd", "-m", os.environ['BUILDER_NAME']])

# Fetch package dependencies for the build
print("2. Installing package dependencies within LXC container")
common.install_common_dependendencies(container)

print("2.1 Install .DEB build support packages")
common.run_command(container, [os.environ["REPO_TOOL"], "install", "-y", "dpkg-dev"])
common.run_command(container, [os.environ["REPO_TOOL"], "install", "-y", "libdistro-info-perl"])
common.run_command(container, [os.environ["REPO_TOOL"], "install", "-y", "dh-make"])
common.run_command(container, [os.environ["REPO_TOOL"], "install", "-y", "dh-systemd"])
common.run_command(container, [os.environ["REPO_TOOL"], "install", "-y", "dh-autoreconf"])
common.run_command(container, [os.environ["REPO_TOOL"], "install", "-y", "git-buildpackage"])

print("2.2 Add more dependencies")
common.run_command(container, [os.environ["REPO_TOOL"], "install", "-y", "libnetfilter-acct-dev"])
common.run_command(container, [os.environ["REPO_TOOL"], "install", "-y", "libcups2-dev"])

print ("3. Run install-required-packages scriptlet")
common.run_command(container, ["wget", "-T", "15", "-O", "%s/.install-required-packages.sh" % build_path, "https://raw.githubusercontent.com/netdata/netdata-demo-site/master/install-required-packages.sh"])
common.run_command(container, ["bash", "%s/.install-required-packages.sh" % build_path, "netdata", "--dont-wait", "--non-interactive"])

friendly_version=""
dest_archive=""
download_url=""
tag = None
friendly_version, tag = common.fetch_version(os.environ['BUILD_VERSION'])

tar_file="%s/netdata-%s.tar.gz" % (os.path.dirname(dest_archive), friendly_version)

print("5. I will be building version '%s' of netdata." % os.environ['BUILD_VERSION'])
dest_archive="%s/netdata-%s.tar.gz" % (build_path, friendly_version)

if str(os.environ["BUILD_STRING"]).count("debian/jessie") == 1:
    print("5.1 We are building for Jessie, adjusting control file")
    common.run_command_in_host(['sudo', 'rm', 'contrib/debian/control'])
    common.run_command_in_host(['sudo', 'cp', 'contrib/debian/control.jessie', 'contrib/debian/control'])

common.prepare_version_source(dest_archive, friendly_version, tag=tag)

print("6. Installing build.sh script to build path")
common.run_command_in_host(['sudo', 'cp', '.travis/package_management/build.sh', "%s/%s/build.sh" % (os.environ['LXC_CONTAINER_ROOT'], build_path)])
common.run_command_in_host(['sudo', 'chmod', '777', "%s/%s/build.sh" % (os.environ['LXC_CONTAINER_ROOT'], build_path)])
common.run_command_in_host(['sudo', 'ln', '-sf', 'contrib/debian', 'debian'])

print("Done!")
