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
# Author  : Pavlos Emm. Katsoulakis <paul@netdata.cloud>

import os
import sys
import lxc

def run_command(command):
    print ("Running command: %s" % command)
    command_result = container.attach_wait(lxc.attach_run_command, command)

    if command_result != 0:
        raise Exception("Command failed with exit code %d" % command_result)

if len(sys.argv) != 2:
    print ('You need to provide a container name to get things started')
    sys.exit(1)
container_name=sys.argv[1]

# Setup the container object
print ("Defining container %s" % container_name)
container = lxc.Container(container_name)
if container.defined:
    raise Exception("Container %s already exists" % container_name)

# Create the container rootfs
if not container.create("download", lxc.LXC_CREATE_QUIET, {"dist": os.environ["BUILD_DISTRO"],
                                                   "release": os.environ["BUILD_RELEASE"],
                                                   "arch": os.environ["BUILD_ARCH"]}):
    raise Exception("Failed to create the container rootfs")
print ("Container %s was successfully created, starting it up" % container_name)

# Start the container
if not container.start():
    raise Exception("Failed to start the container")

if not container.running or not container.state == "RUNNING":
    raise Exception('Container %s is not running, configuration process aborted ' % container_name)

# Wait for connectivity
print ("Waiting for container connectivity to start configuration sequence")
if not container.get_ips(timeout=30):
    raise Exception("Timeout while waiting for container")

# Run the required activities now
# 1. Create the builder user
print ("1. Adding user %s" % os.environ['BUILDER_NAME'])
run_command(["useradd", os.environ['BUILDER_NAME']])

# Fetch wget to retrieve the source
print ("2. Installing package dependencies within LXC container")
run_command(["apt-get", "install", "-y", "wget"])
run_command(["apt-get", "install", "-y", "sudo"])
run_command(["apt-get", "install", "-y", "dh-make"])
run_command(["apt-get", "install", "-y", "dh-systemd"])
run_command(["apt-get", "install", "-y", "bzr-builddeb"])
run_command(["apt-get", "install", "-y", "zlib1g-dev"])
run_command(["apt-get", "install", "-y", "uuid-dev"])
run_command(["apt-get", "install", "-y", "libmnl-dev"])
run_command(["apt-get", "install", "-y", "gcc"])
run_command(["apt-get", "install", "-y", "make"])
run_command(["apt-get", "install", "-y", "git"])
run_command(["apt-get", "install", "-y", "autoconf"])
run_command(["apt-get", "install", "-y", "autoconf-archive"])
run_command(["apt-get", "install", "-y", "autogen"])
run_command(["apt-get", "install", "-y", "automake"])
run_command(["apt-get", "install", "-y", "pkg-config"])
run_command(["apt-get", "install", "-y", "curl"])

# Download the source
dest_archive="/home/%s/netdata-%s.tar.gz" % (os.environ['BUILDER_NAME'],os.environ['BUILD_VERSION'])
release_url="https://github.com/netdata/netdata/releases/download/%s/netdata-%s.tar.gz" % (os.environ['BUILD_VERSION'], os.environ['BUILD_VERSION'])

print ("3. Fetch netdata source (%s -> %s)" % (release_url, dest_archive))
run_command(["sudo", "-u", os.environ['BUILDER_NAME'], "wget", "--output-document=" + dest_archive, release_url])

print ("4. Extracting directory contents to /home " + os.environ['BUILDER_NAME'])
run_command(["sudo", "-u", os.environ['BUILDER_NAME'], "tar", "xf", dest_archive, "-C", "/home/" + os.environ['BUILDER_NAME']])

print ('Done!')
