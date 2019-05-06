#!/usr/bin/python3
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
# Author  : Pavlos Emm. Katsoulakis (paul@netdata.cloud)

import os
import sys
import lxc

if len(sys.argv) != 2:
    print 'You need to provide a container name to get things started'
    sys.exit(1)
container_name=sys.argv[1]

# Setup the container object
container = lxc.Container(container_name)
if container.defined:
    raise Exception("Container %s already exists" % container_name)

# Create the container rootfs
if not container.create("download", lxc.LXC_CREATE_QUIET, {"dist": os.environ["BUILD_DISTRO"],
                                                   "release": os.environ["BUILD_RELEASE"],
                                                   "arch": os.environ["BUILD_ARCH"]}):
    raise Exception("Failed to create the container rootfs")

# Start the container
if not container.start():
    raise Exception("Failed to start the container")

if not container.running or not container.state == "RUNNING":
    raise Exception('Container %s is not running, configuration process aborted ' % container_name)

# Wait for connectivity
if not container.get_ips(timeout=30):
    continue

# Run the required activities now
# 1. Create the builder user
container.attach_wait(lxc.attach_run_command,
                      ["useradd", os.environ['BUILDER_NAME'])

container.attach_wait(lxc.attach_run_command,
                      ["echo", "'%_topdir %(echo /home/%s)/rpmbuild' > /home/%s/.rpmmacros" % (os.environ['BUILDER_NAME'], os.environ['BUILDER_NAME'])])

# Download the source
dest_archive="/home/%s/rpmbuild/SOURCES/netdata-%s.tar.gz" % (os.environ['BUILDER_NAME'],os.environ['BUILD_VERSION'])
release_url="https://github.com/netdata/netdata/releases/download/%s/netdata-%s.tar.gz" % (os.environ['BUILD_VERSION'], os.environ['BUILD_VERSION'])
container.attach_wait(lxc.attach_run_command,
                      ["wget", "-O", dest_archive, release_url])

# Extract the spec file in place
spec_file="/home/%s/rpmbuild/SPECS/netdata.spec" % os.environ['BUILDER_NAME']
container.attach_wait(lxc.attach_run_command,
                      ["tar", "-Oxvf", dest_archive, "netdata-%s/netdata.spec > %s" % (os.environ['BUILD_VERSION'], spec_file)])

print 'Done!'
