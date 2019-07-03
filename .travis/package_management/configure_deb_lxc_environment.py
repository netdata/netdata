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
common.run_command(container, ["apt-get", "update", "-y"])
common.run_command(container, ["apt-get", "install", "-y", "sudo"])
common.run_command(container, ["apt-get", "install", "-y", "wget"])
common.run_command(container, ["apt-get", "install", "-y", "bash"])
common.run_command(container, ["wget", "-T", "15", "-O", "~/.install-required-packages.sh", "https://raw.githubusercontent.com/netdata/netdata-demo-site/master/install-required-packages.sh"])
common.run_command(container, ["bash", "~/.install-required-packages.sh", "netdata", "--dont-wait", "--non-interactive"])

# Download the source
dest_archive="/home/%s/netdata-%s.tar.gz" % (os.environ['BUILDER_NAME'],os.environ['BUILD_VERSION'])
release_url="https://github.com/netdata/netdata/releases/download/%s/netdata-%s.tar.gz" % (os.environ['BUILD_VERSION'], os.environ['BUILD_VERSION'])
print("3. Fetch netdata source (%s -> %s)" % (release_url, dest_archive))
common.run_command(container, ["sudo", "-u", os.environ['BUILDER_NAME'], "wget", "-T", "15", "--output-document=" + dest_archive, release_url])

print("4. Extracting directory contents to /home " + os.environ['BUILDER_NAME'])
common.run_command(container, ["sudo", "-u", os.environ['BUILDER_NAME'], "tar", "xf", dest_archive, "-C", "/home/" + os.environ['BUILDER_NAME']])

print("Done!")
