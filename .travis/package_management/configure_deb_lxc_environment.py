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

# Run the required activities now
# 1. Create the builder user
print("1. Adding user %s" % os.environ['BUILDER_NAME'])
common.run_command(container, ["useradd", "-m", os.environ['BUILDER_NAME']])

# Fetch package dependencies for the build
print("2. Installing package dependencies within LXC container")
common.install_common_dependendencies()

print ("3. Run install-required-packages scriptlet")
common.run_command(container, ["wget", "-T", "15", "-O", "/home/%s/.install-required-packages.sh" % (os.environ['BUILDER_NAME']), "https://raw.githubusercontent.com/netdata/netdata-demo-site/master/install-required-packages.sh"])
common.run_command(container, ["bash", "/home/%s/.install-required-packages.sh" % (os.environ['BUILDER_NAME']), "netdata", "--dont-wait", "--non-interactive"])

friendly_version=""
dest_archive=""
download_url=""
tag = None

# TODO: Checksum validations
if str(os.environ['BUILD_VERSION']).count(".latest") == 1:
    version_list=str(os.environ['BUILD_VERSION']).replace('v', '').split('.')
    friendly_version='.'.join(version_list[0:3]) + "." + version_list[3]
else:
    friendly_version = os.environ['BUILD_VERSION'].replace('v', '')
    tag = friendly_version # Go to stable tag

tar_file="%s/netdata-%s.tar.gz" % (os.path.dirname(dest_archive), friendly_version)

print("5. I will be building version '%s' of netdata." % os.environ['BUILD_VERSION'])
dest_archive="/home/%s/netdata-%s.tar.gz" % (os.environ['BUILDER_NAME'], friendly_version)

common.prepare_version_source(dest_archive, friendly_version, tag=tag)

print("Done!")
