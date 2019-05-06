#!/usr/bin/python3
#
# Prepare the build environment within the container
# The script attaches to the running container and does the following:
# 1) Create the builder user
# 2) Prepare the environment for RPM build
#
# Copyright: SPDX-License-Identifier: GPL-3.0-or-later
#
# Author  : Pavlos Emm. Katsoulakis (paul@netdata.cloud)

import os
import sys
import lxc

if len(sys.argv) != 1:
    print 'You need to provide a container name to get things started'
    sys.exit(1)

container_name=sys.argv[0]
container_list = lxc.list_containers(as_object=True)
c = [i for i in container_list if i.name == container_name]
if len(c) != 1:
    raise Exception('Unexpected number of containers found with name %s (found %d, expected 1) ' % (container_name, len(c)))
container = c[0]

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
