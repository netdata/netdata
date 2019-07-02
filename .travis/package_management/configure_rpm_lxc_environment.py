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
print("2. Installing package dependencies within LXC container")
if str(os.environ["REPO_TOOL"]).count("zypper") == 1:
    common.run_command(container, [os.environ["REPO_TOOL"], "clean", "-a"])
    common.run_command(container, [os.environ["REPO_TOOL"], "--no-gpg-checks", "update", "-y"])
    common.run_command(container, [os.environ["REPO_TOOL"], "install", "-y", "json-glib-devel"])

elif str(os.environ["REPO_TOOL"]).count("yum") == 1:
    common.run_command(container, [os.environ["REPO_TOOL"], "install", "-y", "json-c-devel"])
    common.run_command(container, [os.environ["REPO_TOOL"], "clean", "all"])
    common.run_command(container, [os.environ["REPO_TOOL"], "update", "-y"])
    common.run_command(container, [os.environ["REPO_TOOL"], "install", "-y", "epel-release"])
else:
    common.run_command(container, [os.environ["REPO_TOOL"], "install", "-y", "json-c-devel"])
    common.run_command(container, [os.environ["REPO_TOOL"], "update", "-y"])

common.run_command(container, [os.environ["REPO_TOOL"], "install", "-y", "sudo"])
common.run_command(container, [os.environ["REPO_TOOL"], "install", "-y", "wget"])
common.run_command(container, [os.environ["REPO_TOOL"], "install", "-y", "bash"])
common.run_command(container, [os.environ["REPO_TOOL"], "install", "-y", "freeipmi-devel"])
common.run_command(container, [os.environ["REPO_TOOL"], "install", "-y", "cups-devel"])

# Exceptional cases, not available everywhere
#

# Not on Centos-7
if os.environ["BUILD_STRING"].count("el/7") <= 0:
    common.run_command(container, [os.environ["REPO_TOOL"], "install", "-y", "libnetfilter_acct-devel"])

# Not on Centos-6
if os.environ["BUILD_STRING"].count("el/6") <= 0:
    common.run_command(container, [os.environ["REPO_TOOL"], "install", "-y", "autoconf-archive"])

common.run_command(container, ["wget", "-T", "15", "-O", "/home/%s/.install-required-packages.sh" % (os.environ['BUILDER_NAME']), "https://raw.githubusercontent.com/netdata/netdata-demo-site/master/install-required-packages.sh"])
common.run_command(container, ["bash", "/home/%s/.install-required-packages.sh" % (os.environ['BUILDER_NAME']), "netdata", "--dont-wait", "--non-interactive"])

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

# TODO: Checksum validations
if str(os.environ['BUILD_VERSION']).count(".latest") == 1:
    version_list=str(os.environ['BUILD_VERSION']).replace('v', '').split('.')
    rpm_friendly_version='.'.join(version_list[0:3]) + "." + version_list[3]

    print("Building latest nightly version of netdata..(%s)" % os.environ['BUILD_VERSION'])
    dest_archive="/home/%s/rpmbuild/SOURCES/netdata-%s.tar.gz" % (os.environ['BUILDER_NAME'], rpm_friendly_version)

    print("5. Preparing local latest implementation tarball for version %s" % rpm_friendly_version)
    tar_file = os.environ['LXC_CONTAINER_ROOT'] + dest_archive

    print("5.1 Tagging the code with latest version: %s" % rpm_friendly_version)
    common.run_command_in_host(['git', 'tag', '-a', rpm_friendly_version, '-m', 'Tagging while packaging on %s' % os.environ["CONTAINER_NAME"]])

    print("5.2 Run autoreconf -ivf")
    common.run_command_in_host(['autoreconf', '-ivf'])

    print("5.3 Run configure")
    common.run_command_in_host(['./configure', '--with-math', '--with-zlib', '--with-user=netdata'])

    print("5.4 Run make dist")
    common.run_command_in_host(['make', 'dist'])

    print("5.5 Copy generated tarbal to desired path")
    if os.path.exists('netdata-%s.tar.gz' % rpm_friendly_version):
        common.run_command_in_host(['sudo', 'cp', 'netdata-%s.tar.gz' % rpm_friendly_version, tar_file])

        print("5.6 Fixing permissions on tarball")
        common.run_command_in_host(['sudo', 'chmod', '777', tar_file])
    else:
        print("I could not find (%s) on the disk, stopping the build. Kindly check the logs and try again" % 'netdata-%s.tar.gz' % rpm_friendly_version)
        sys.exit(1)

    # Extract the spec file in place
    print("6. Extract spec file from the source")
    common.run_command_in_host(['sudo', 'cp', 'netdata.spec', os.environ['LXC_CONTAINER_ROOT'] + spec_file])
    common.run_command_in_host(['sudo', 'chmod', '777', os.environ['LXC_CONTAINER_ROOT'] + spec_file])

    print("7. Temporary hack: Change Source0 to %s on spec file %s" % (dest_archive, spec_file))
    common.replace_tag("Source0", os.environ['LXC_CONTAINER_ROOT'] + spec_file, tar_file)
else:
    rpm_friendly_version = os.environ['BUILD_VERSION']

    print("Building latest stable version of netdata.. (%s)" % os.environ['BUILD_VERSION'])
    dest_archive="/home/%s/rpmbuild/SOURCES/netdata-%s.tar.gz" % (os.environ['BUILDER_NAME'],os.environ['BUILD_VERSION'])
    download_url="https://github.com/netdata/netdata/releases/download/%s/netdata-%s.tar.gz" % (os.environ['BUILD_VERSION'], os.environ['BUILD_VERSION'])

    print("5. Fetch netdata source into the repo structure(%s -> %s)" % (download_url, dest_archive))
    tar_file="%s/netdata-%s.tar.gz" % (os.path.dirname(dest_archive), rpm_friendly_version)
    common.run_command(container, ["sudo", "-u", os.environ['BUILDER_NAME'], "wget", "-T", "15", "--output-document=" + dest_archive, download_url])

    print("6.Extract spec file from the source")
    common.run_command(container, ["sudo", "-u", os.environ['BUILDER_NAME'], "tar", "--to-command=cat > %s" % spec_file, "-xvf", dest_archive, "netdata-%s/netdata.spec" % os.environ['BUILD_VERSION']])

    print("7. Temporary hack: Adjust version string on the spec file (%s) to %s and Source0 to %s" % (os.environ['LXC_CONTAINER_ROOT'] + spec_file, rpm_friendly_version, download_url))
    common.replace_tag("Version", os.environ['LXC_CONTAINER_ROOT'] + spec_file, rpm_friendly_version)
    common.replace_tag("Source0", os.environ['LXC_CONTAINER_ROOT'] + spec_file, tar_file)

print('Done!')
