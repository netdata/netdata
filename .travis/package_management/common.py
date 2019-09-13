#
#
# Python library with commonly used functions within the package management scope
#
# Author   : Pavlos Emm. Katsoulakis <paul@netdata.cloud>

import lxc
import subprocess
import os
import sys
import tempfile
import shutil

def fetch_version(orig_build_version):
    tag = None
    friendly_version = ""

    # TODO: Checksum validations
    if str(orig_build_version).count(".latest") == 1:
        version_list=str(orig_build_version).replace('v', '').split('.')
        minor = version_list[3] if int(version_list[2]) == 0 else (version_list[2] + version_list[3])
        friendly_version='.'.join(version_list[0:2]) + "." + minor
    else:
        friendly_version = orig_build_version.replace('v', '')
        tag = friendly_version # Go to stable tag
    print("Version set to %s from %s" % (friendly_version, orig_build_version))

    return friendly_version, tag

def replace_tag(tag_name, spec, new_tag_content):
    print("Fixing tag %s in %s" % (tag_name, spec))

    ifp = open(spec, "r")
    config = ifp.readlines()
    ifp.close()

    source_line = -1
    for line in config:
        if str(line).count(tag_name + ":") > 0:
            source_line = config.index(line)
            print("Found line: %s in item %d" % (line, source_line))
            break

    if source_line >= 0:
        print("Replacing line %s with %s in spec file" %(config[source_line], new_tag_content))
        config[source_line] = "%s: %s\n" % (tag_name, new_tag_content)
        config_str = ''.join(config)
        ofp = open(spec, 'w')
        ofp.write(config_str)
        ofp.close()

def run_command(container, command):
    print("Running command: %s" % command)
    command_result = container.attach_wait(lxc.attach_run_command, command)

    if command_result != 0:
        raise Exception("Command failed with exit code %d" % command_result)

def run_command_in_host(cmd, cwd=None):
    print("Issue command in host: %s, cwd:%s" % (str(cmd), str(cwd)))

    proc = subprocess.Popen(cmd, stdout=subprocess.PIPE, stderr=subprocess.PIPE, cwd=cwd)
    o, e = proc.communicate()
    print('Output: ' + o.decode('ascii'))
    print('Error: '  + e.decode('ascii'))
    print('code: ' + str(proc.returncode))

def prepare_repo(container):
    if str(os.environ["REPO_TOOL"]).count("zypper") == 1:
        run_command(container, [os.environ["REPO_TOOL"], "clean", "-a"])
        run_command(container, [os.environ["REPO_TOOL"], "--no-gpg-checks", "update", "-y"])

    elif str(os.environ["REPO_TOOL"]).count("yum") == 1:
        run_command(container, [os.environ["REPO_TOOL"], "clean", "all"])
        run_command(container, [os.environ["REPO_TOOL"], "update", "-y"])

        if os.environ["BUILD_STRING"].count("el/7") == 1 and os.environ["BUILD_ARCH"].count("i386") == 1:
            print ("Skipping epel-release install for %s-%s" % (os.environ["BUILD_STRING"], os.environ["BUILD_ARCH"]))
        else:
            run_command(container, [os.environ["REPO_TOOL"], "install", "-y", "epel-release"])

    elif str(os.environ["REPO_TOOL"]).count("apt-get") == 1:
        run_command(container, [os.environ["REPO_TOOL"], "update", "-y"])
    else:
        run_command(container, [os.environ["REPO_TOOL"], "update", "-y"])

    run_command(container, [os.environ["REPO_TOOL"], "install", "-y", "sudo"])
    run_command(container, [os.environ["REPO_TOOL"], "install", "-y", "wget"])
    run_command(container, [os.environ["REPO_TOOL"], "install", "-y", "bash"])

def install_common_dependendencies(container):
    if str(os.environ["REPO_TOOL"]).count("zypper") == 1:
        run_command(container, [os.environ["REPO_TOOL"], "install", "-y", "gcc-c++"])
        run_command(container, [os.environ["REPO_TOOL"], "install", "-y", "json-glib-devel"])
        run_command(container, [os.environ["REPO_TOOL"], "install", "-y", "freeipmi-devel"])
        run_command(container, [os.environ["REPO_TOOL"], "install", "-y", "cups-devel"])
        run_command(container, [os.environ["REPO_TOOL"], "install", "-y", "snappy-devel"])
        run_command(container, [os.environ["REPO_TOOL"], "install", "-y", "protobuf-devel"])
        run_command(container, [os.environ["REPO_TOOL"], "install", "-y", "protobuf-c"])

    elif str(os.environ["REPO_TOOL"]).count("yum") == 1:
        run_command(container, [os.environ["REPO_TOOL"], "install", "-y", "gcc-c++"])
        run_command(container, [os.environ["REPO_TOOL"], "install", "-y", "json-c-devel"])
        run_command(container, [os.environ["REPO_TOOL"], "install", "-y", "freeipmi-devel"])
        run_command(container, [os.environ["REPO_TOOL"], "install", "-y", "cups-devel"])
        run_command(container, [os.environ["REPO_TOOL"], "install", "-y", "snappy-devel"])
        run_command(container, [os.environ["REPO_TOOL"], "install", "-y", "protobuf-devel"])
        run_command(container, [os.environ["REPO_TOOL"], "install", "-y", "protobuf-c-devel"])
        run_command(container, [os.environ["REPO_TOOL"], "install", "-y", "protobuf-compiler"])

    elif str(os.environ["REPO_TOOL"]).count("apt-get") == 1:
        run_command(container, [os.environ["REPO_TOOL"], "install", "-y", "g++"])
        run_command(container, [os.environ["REPO_TOOL"], "install", "-y", "libipmimonitoring-dev"])
        run_command(container, [os.environ["REPO_TOOL"], "install", "-y", "libjson-c-dev"])
        run_command(container, [os.environ["REPO_TOOL"], "install", "-y", "libcups2-dev"])
        run_command(container, [os.environ["REPO_TOOL"], "install", "-y", "libsnappy-dev"])
        run_command(container, [os.environ["REPO_TOOL"], "install", "-y", "libprotobuf-dev"])
        run_command(container, [os.environ["REPO_TOOL"], "install", "-y", "libprotoc-dev"])
        run_command(container, [os.environ["REPO_TOOL"], "install", "-y", "protobuf-compiler"])
        if os.environ["BUILD_STRING"].count("debian/jessie") == 1:
            run_command(container, [os.environ["REPO_TOOL"], "install", "-y", "snappy"])
    else:
        run_command(container, [os.environ["REPO_TOOL"], "install", "-y", "gcc-c++"])
        run_command(container, [os.environ["REPO_TOOL"], "install", "-y", "cups-devel"])
        run_command(container, [os.environ["REPO_TOOL"], "install", "-y", "freeipmi-devel"])
        run_command(container, [os.environ["REPO_TOOL"], "install", "-y", "json-c-devel"])
        run_command(container, [os.environ["REPO_TOOL"], "install", "-y", "snappy-devel"])
        run_command(container, [os.environ["REPO_TOOL"], "install", "-y", "protobuf-devel"])
        run_command(container, [os.environ["REPO_TOOL"], "install", "-y", "protobuf-c-devel"])
        run_command(container, [os.environ["REPO_TOOL"], "install", "-y", "protobuf-compiler"])

    if os.environ["BUILD_STRING"].count("el/6") <= 0:
        run_command(container, [os.environ["REPO_TOOL"], "install", "-y", "autogen"])

def prepare_version_source(dest_archive, pkg_friendly_version, tag=None):
    print(".0 Preparing local implementation tarball for version %s" % pkg_friendly_version)
    tar_file = os.environ['LXC_CONTAINER_ROOT'] + dest_archive

    print(".0 Copy repo to prepare it for tarball generation")
    tmp_src = tempfile.mkdtemp(prefix='netdata-source-')
    run_command_in_host(['cp', '-r', '.', tmp_src])

    if tag is not None:
        print(".1 Checking out tag %s" % tag)
        run_command_in_host(['git', 'fetch', '--all'], tmp_src)

        # TODO: Keep in mind that tricky 'v' there, needs to be removed once we clear our versioning scheme
        run_command_in_host(['git', 'checkout', 'v%s' % pkg_friendly_version], tmp_src)

    print(".2 Tagging the code with version: %s" % pkg_friendly_version)
    run_command_in_host(['git', 'tag', '-a', pkg_friendly_version, '-m', 'Tagging while packaging on %s' % os.environ["CONTAINER_NAME"]], tmp_src)

    print(".3 Run autoreconf -ivf")
    run_command_in_host(['autoreconf', '-ivf'], tmp_src)

    print(".4 Run configure")
    run_command_in_host(['./configure', '--prefix=/usr', '--sysconfdir=/etc', '--localstatedir=/var', '--libdir=/usr/lib', '--libexecdir=/usr/libexec', '--with-math', '--with-zlib', '--with-user=netdata'], tmp_src)

    print(".5 Run make dist")
    run_command_in_host(['make', 'dist'], tmp_src)

    print(".6 Copy generated tarbal to desired path")
    generated_tarball = '%s/netdata-%s.tar.gz' % (tmp_src, pkg_friendly_version)

    if os.path.exists(generated_tarball):
        run_command_in_host(['sudo', 'cp', generated_tarball, tar_file])

        print(".7 Fixing permissions on tarball")
        run_command_in_host(['sudo', 'chmod', '777', tar_file])

        print(".8 Returning to original directory, removing temp");
        shutil.rmtree(tmp_src)
    else:
        print("I could not find (%s) on the disk, stopping the build. Kindly check the logs and try again" % generated_tarball)
        sys.exit(1)
