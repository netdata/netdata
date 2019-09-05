#!/usr/bin/env python3
#
# This script is responsible for running the RPM build on the running container
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

# Load the container, break if its not there
print("Starting up container %s" % container_name)
container = lxc.Container(container_name)
if not container.defined:
    raise Exception("Container %s does not exist!" % container_name)

# Check if the container is running, attempt to start it up in case its not running
if not container.running or not container.state == "RUNNING":
    print('Container %s is not running, attempt to start it up' % container_name)

    # Start the container
    if not container.start():
        raise Exception("Failed to start the container")

    if not container.running or not container.state == "RUNNING":
        raise Exception('Container %s is not running, configuration process aborted ' % container_name)

# Wait for connectivity
if not container.get_ips(timeout=30):
    raise Exception("Timeout while waiting for container")

build_path = "/home/%s" % os.environ['BUILDER_NAME']

print("Setting up EMAIL and DEBFULLNAME variables required by the build tools")
os.environ["EMAIL"] = "bot@netdata.cloud"
os.environ["DEBFULLNAME"] = "Netdata builder"

# Run the build process on the container
new_version, tag = common.fetch_version(os.environ['BUILD_VERSION'])
print("Starting DEB build process for version %s" % new_version)

netdata_tarball = "%s/netdata-%s.tar.gz" % (build_path, new_version)
unpacked_netdata = netdata_tarball.replace(".tar.gz", "")

print("Extracting tarball %s" % netdata_tarball)
common.run_command(container, ["sudo", "-u", os.environ['BUILDER_NAME'], "tar", "xf", netdata_tarball, "-C", build_path])

print("Checking version consistency")
since_version = os.environ["LATEST_RELEASE_VERSION"]
if str(since_version).replace('v', '') == str(new_version) and str(new_version).count('.') == 2:
    s = since_version.split('.')
    prev = str(int(s[1]) - 1)
    since_version = s[0] + '.' + prev + s[2]
    print("We seem to be building a new stable release, reduce by one since_version option. New since_version:%s" % since_version)

print("Fixing changelog tags")
changelog_in_host = "contrib/debian/changelog"
common.run_command_in_host(['sed', '-i', 's/PREVIOUS_PACKAGE_VERSION/%s-1/g' % since_version.replace("v", ""), changelog_in_host])
common.run_command_in_host(['sed', '-i', 's/PREVIOUS_PACKAGE_DATE/%s/g' % os.environ["LATEST_RELEASE_DATE"], changelog_in_host])

print("Executing gbp dch command..")
common.run_command_in_host(['gbp', 'dch', '--release', '--ignore-branch', '--spawn-editor=snapshot', '--since=%s' % since_version, '--new-version=%s' % new_version])

print("Copying over changelog to the destination machine")
common.run_command_in_host(['sudo', 'cp', 'debian/changelog', "%s/%s/netdata-%s/contrib/debian/" % (os.environ['LXC_CONTAINER_ROOT'], build_path, new_version)])

print("Running debian build script since %s" % since_version)
common.run_command(container, ["sudo", "-u", os.environ['BUILDER_NAME'], "%s/build.sh" % build_path, unpacked_netdata, new_version])

print("Listing contents on build path")
common.run_command(container, ["sudo", "-u", os.environ['BUILDER_NAME'], "ls", "-ltr", build_path])

print('Done!')
