import os
from pathlib import Path

import dagger


_ALPINE_COMMON_PACKAGES = [
    "alpine-sdk",
    "autoconf",
    "automake",
    "bash",
    "binutils",
    "bison",
    "cmake",
    "curl",
    "curl-static",
    "elfutils-dev",
    "flex",
    "gcc",
    "git",
    "gnutls-dev",
    "gzip",
    "jq",
    "libelf-static",
    "libmnl-dev",
    "libmnl-static",
    "libtool",
    "libuv-dev",
    "libuv-static",
    "lz4-dev",
    "lz4-static",
    "make",
    "mongo-c-driver-dev",
    "mongo-c-driver-static",
    "musl-fts-dev",
    "ncurses",
    "ncurses-static",
    "netcat-openbsd",
    "ninja",
    "openssh",
    "pcre2-dev",
    "pkgconfig",
    "protobuf-dev",
    "snappy-dev",
    "snappy-static",
    "util-linux-dev",
    "wget",
    "xz",
    "yaml-dev",
    "yaml-static",
    "zlib-dev",
    "zlib-static",
    "zstd-dev",
    "zstd-static",
]


def build_alpine_3_18(client, platform):
    ctr = client.container(platform=platform).from_("alpine:3.18")

    pkgs = [pkg for pkg in _ALPINE_COMMON_PACKAGES]

    ctr = (
        ctr.with_exec(["apk", "add", "--no-cache"] + pkgs)
    )

    return ctr


def build_alpine_3_19(client, platform: dagger.Platform):
    ctr = client.container(platform=platform).from_("alpine:3.19")

    pkgs = [pkg for pkg in _ALPINE_COMMON_PACKAGES]

    ctr = (
        ctr.with_exec(["apk", "add", "--no-cache"] + pkgs)
    )

    return ctr


def static_build_openssl(client: dagger.Client, ctr: dagger.Container):
    tree = (
        client.git(url="https://github.com/openssl/openssl", keep_git_dir=True)
              .tag("openssl-3.1.4").tree()
    )

    #
    # TODO: verify 32-bit builds
    #
    ctr = (
        ctr.with_directory("/openssl", tree)
           .with_workdir("/openssl")
           .with_env_variable("CFLAGS", "-fno-lto -pipe")
           .with_env_variable("LDFLAGS", "-static")
           .with_env_variable("PKG_CONFIG", "pkg-config --static")
           .with_exec(["sed", "-i", "s/disable('static', 'pic', 'threads');/disable('static', 'pic');/", "Configure"])
           .with_exec(["./config", "-static", "threads", "no-tests", "--prefix=/openssl-static", "--openssldir=/opt/netdata/etc/ssl"])
           .with_exec(["make", "V=1", "-j", str(os.cpu_count())])
           .with_exec(["make", "V=1", "-j", str(os.cpu_count()), "install_sw"])
           .with_exec(["ln", "-s", "/openssl-static/lib", "/openssl-static/lib64"])
           .with_exec(["perl", "configdata.pm", "--dump"])
    )

    return ctr


def static_build_bash(client: dagger.Client, ctr: dagger.Container):
    tree = (
        client.git(url="https://git.savannah.gnu.org/git/bash.git", keep_git_dir=True)
              .tag("bash-5.1").tree()
    )

    ctr = (
        ctr.with_directory("/bash", tree)
           .with_workdir("/bash")
           .with_env_variable("CFLAGS", "-pipe")
           .with_env_variable("LDFLAGS", "")
           .with_env_variable("PKG_CONFIG", "pkg-config --static")
           .with_env_variable("PKG_CONFIG_PATH", "/openssl-static/lib64/pkgconfig")
           .with_exec([
                "./configure", "--prefix", "/opt/netdata",
                               "--without-bash-malloc",
                               "--enable-static-link",
                               "--enable-net-redirections",
                               "--enable-array-variables",
                               "--disable-progcomp",
                               "--disable-profiling",
                               "--disable-nls",
                               "--disable-dependency-tracking",
           ])
           .with_exec(["echo", "-e", "all:\nclean:\ninstall:\n", ">", "examples/loadables/Makefile"])
           .with_exec(["make", "clean"])
           # see: https://gitweb.gentoo.org/repo/gentoo.git/tree/app-shells/bash/files/bash-5.1-parallel_make.patch?id=4c2ebbf4b8bc660beb98cc2d845c73375d6e4f50
           .with_exec(["make", "V=1", "-j", "2", "install"])
           .with_exec(["strip", "/opt/netdata/bin/bash"])
    )

    return ctr

def static_build_curl(client: dagger.Client, ctr: dagger.Container):
    tree = (
        client.git(url="https://github.com/curl/curl", keep_git_dir=True)
              .tag("curl-8_4_0").tree()
    )

    ctr = (
        ctr.with_directory("/curl", tree)
           .with_workdir("/curl")
           .with_env_variable("CFLAGS", "-I/openssl-static/include -pipe")
           .with_env_variable("LDFLAGS", "-static -L/openssl-static/lib64")
           .with_env_variable("PKG_CONFIG", "pkg-config --static")
           .with_env_variable("PKG_CONFIG_PATH", "/openssl-static/lib64/pkgconfig")
           .with_exec(["autoreconf", "-ifv"])
           .with_exec([
                "./configure", "--prefix=/curl-static",
                               "--enable-optimize",
                               "--disable-shared",
                               "--enable-static",
                               "--enable-http",
                               "--disable-ldap",
                               "--disable-ldaps",
                               "--enable-proxy",
                               "--disable-dict",
                               "--disable-telnet",
                               "--disable-tftp",
                               "--disable-pop3",
                               "--disable-imap",
                               "--disable-smb",
                               "--disable-smtp",
                               "--disable-gopher",
                               "--enable-ipv6",
                               "--enable-cookies",
                               "--with-ca-fallback",
                               "--with-openssl",
                               "--disable-dependency-tracking",
           ])
           .with_exec(["sed", "-i", "-e", "s/LDFLAGS =/LDFLAGS = -all-static/", "src/Makefile"])
           .with_exec(["make", "clean"])
           .with_exec(["make", "V=1", "-j", str(os.cpu_count()), "install"])
           .with_exec(["cp", "/curl-static/bin/curl", "/opt/netdata/bin/curl"])
           .with_exec(["strip", "/opt/netdata/bin/curl"])
    )

    return ctr


def static_build_ioping(client: dagger.Client, ctr: dagger.Container):
    tree = (
        client.git(url="https://github.com/koct9i/ioping", keep_git_dir=True)
              .tag("v1.3").tree()
    )

    ctr = (
        ctr.with_directory("/ioping", tree)
           .with_workdir("/ioping")
           .with_env_variable("CFLAGS", "-static -pipe")
           .with_exec(["mkdir", "-p", "/opt/netdata/usr/libexec/netdata/plugins.d"])
           .with_exec(["make", "V=1"])
           .with_exec(["install", "-o", "root", "-g", "root", "-m", "4750", "ioping", "/opt/netdata/usr/libexec/netdata/plugins.d"])
           .with_exec(["strip", "/opt/netdata/usr/libexec/netdata/plugins.d/ioping"])
    )

    return ctr


def static_build_libnetfilter_acct(client: dagger.Client, ctr: dagger.Container):
    tree = (
        client.git(url="git://git.netfilter.org/libnetfilter_acct", keep_git_dir=True)
              .tag("libnetfilter_acct-1.0.3").tree()
    )
    
    ctr = (
        ctr.with_directory("/libnetfilter_acct", tree)
           .with_workdir("/libnetfilter_acct")
           .with_env_variable("CFLAGS", "-static -I/usr/include/libmnl -pipe")
           .with_env_variable("LDFLAGS", "-static -L/usr/lib -lmnl")
           .with_env_variable("PKG_CONFIG", "pkg-config --static")
           .with_env_variable("PKG_CONFIG_PATH", "/usr/lib/pkgconfig")
           .with_exec(["autoreconf", "-ifv"])
           .with_exec([
                "./configure", "--prefix=/libnetfilter_acct-static",
                               "--exec-prefix=/libnetfilter_acct-static",
           ])
           .with_exec(["make", "clean"])
           .with_exec(["make", "V=1", "-j", str(os.cpu_count()), "install"])
    )

    return ctr

def static_build_netdata(client: dagger.Client, ctr: dagger.Container):
    CFLAGS = [
        "-ffunction-sections",
        "-fdata-sections",
        "-static",
        "-O2",
        "-funroll-loops",
        "-I/openssl-static/include",
        "-I/libnetfilter_acct-static/include/libnetfilter_acct",
        "-I/curl-local/include/curl",
        "-I/usr/include/libmnl",
        "-pipe"
    ]

    LDFLAGS = [
        "-Wl,--gc-sections",
        "-static",
        "-L/openssl-static/lib64",
        "-L/libnetfilter_acct-static/lib",
        "-lnetfilter_acct",
        "-L/usr/lib",
        "-lmnl",
        "-L/usr/lib",
        "-lzstd",
        "-L/curl-local/lib",
    ]

    PKG_CONFIG = [
        "pkg-config",
        "--static",
    ]

    PKG_CONFIG_PATH = [
        "/openssl-static/lib64/pkgconfig",
        "/libnetfilter_acct-static/lib/pkgconfig",
        "/usr/lib/pkgconfig",
        "/curl-local/lib/pkgconfig",
    ]

    CMAKE_FLAGS=[
        "-DOPENSSL_ROOT_DIR=/openssl-static",
        "-DOPENSSL_LIBRARIES=/openssl-static/lib64",
        "-DCMAKE_INCLUDE_DIRECTORIES_PROJECT_BEFORE=/openssl-static",
        "-DLWS_OPENSSL_INCLUDE_DIRS=/openssl-static/include",
        "-DLWS_OPENSSL_LIBRARIES=/openssl-static/lib64/libssl.a;/openssl-static/lib64/libcrypto.a",
    ]

    NETDATA_INSTALLER_CMD = [
        "./netdata-installer.sh", "--install-prefix", "/opt",
                                  "--dont-wait",
                                  "--dont-start-it",
                                  "--disable-exporting-mongodb",
                                  "--require-cloud",
                                  "--use-system-protobuf",
                                  "--dont-scrub-cflags-even-though-it-may-break-things",
                                  "--one-time-build",
                                  "--enable-lto",
    ]

    ctr = (
        ctr.with_workdir("/netdata")
           .with_env_variable("NETDATA_CMAKE_OPTIONS", "-DCMAKE_BUILD_TYPE=Debug")
           .with_env_variable("CFLAGS", " ".join(CFLAGS))
           .with_env_variable("LDFLAGS", " ".join(LDFLAGS))
           .with_env_variable("PKG_CONFIG", " ".join(PKG_CONFIG))
           .with_env_variable("PKG_CONFIG_PATH", ":".join(PKG_CONFIG_PATH))
           .with_env_variable("CMAKE_FLAGS", " ".join(CMAKE_FLAGS))
           .with_env_variable("EBPF_LIBC", "static")
           .with_env_variable("IS_NETDATA_STATIC_BINARY", "yes")
           .with_exec(NETDATA_INSTALLER_CMD)
    )

    return ctr


def static_build(client, repo_path):
    cmake_build_release_path = os.path.join(repo_path, "cmake-build-release")

    ctr = build_alpine_3_18(client, dagger.Platform("linux/x86_64"))
    ctr = static_build_openssl(client, ctr)
    ctr = static_build_bash(client, ctr)
    ctr = static_build_curl(client, ctr)
    ctr = static_build_ioping(client, ctr)
    ctr = static_build_libnetfilter_acct(client, ctr)

    ctr = (
        ctr.with_directory("/netdata", client.host().directory(repo_path), exclude=[
                f"{cmake_build_release_path}/*",
                "fluent-bit/build",
        ])
    )

    # TODO: link bin/sbin
    
    ctr = static_build_netdata(client, ctr)

    build_dir = ctr.directory('/opt/netdata')
    artifact_dir = os.path.join(Path.home(), f'ci/netdata-static')
    output_task = build_dir.export(artifact_dir)
    return output_task


_CENTOS_COMMON_PACKAGES = [
    "autoconf",
    "autoconf-archive",
    "autogen",
    "automake",
    "bison",
    "bison-devel",
    "cmake",
    "cups-devel",
    "curl",
    "diffutils",
    "elfutils-libelf-devel",
    "findutils",
    "flex",
    "flex-devel",
    "freeipmi-devel",
    "gcc",
    "gcc-c++",
    "git-core",
    "golang",
    "json-c-devel",
    "libyaml-devel",
    "libatomic",
    "libcurl-devel",
    "libmnl-devel",
    "libnetfilter_acct-devel",
    "libtool",
    "libuuid-devel",
    "libuv-devel",
    "libzstd-devel",
    "lm_sensors",
    "lz4-devel",
    "make",
    "ninja-build",
    "openssl-devel",
    "openssl-perl",
    "patch",
    "pcre2-devel",
    "pkgconfig",
    "pkgconfig(libmongoc-1.0)",
    "procps",
    "protobuf-c-devel",
    "protobuf-compiler",
    "protobuf-devel",
    "rpm-build",
    "rpm-devel",
    "rpmdevtools",
    "snappy-devel",
    "systemd-devel",
    "wget",
    "zlib-devel",
]

def build_amazon_linux_2(client, platform):
    ctr = client.container(platform=platform).from_("amazonlinux:2")

    pkgs = [pkg for pkg in _CENTOS_COMMON_PACKAGES]

    ctr = (
        ctr.with_exec(["yum", "update", "-y"])
                .with_exec(["yum", "install", "-y"] + pkgs)
                .with_exec(["yum", "clean", "all"])
                .with_exec(["c_rehash"])
                .with_exec([
                    "mkdir", "-p",
                    "/root/rpmbuild/BUILD",
                    "/root/rpmbuild/RPMS",
                    "/root/rpmbuild/SOURCES",
                    "/root/rpmbuild/SPECS",
                    "/root/rpmbuild/SRPMS",
                ])
    )

    if platform == "linux/x86_64":
        machine = "x86_64"
    elif platform == "linux/arm64":
        machine = "aarch64"
    else:
        raise Exception("Amaxon Linux 2 supports only linux/amd64 and linux/arm64 platforms.")

    repo_path = str(Path(__file__).parent.parent.parent)
    this_path = os.path.join(repo_path, "packaging/dag")

    ctr = (
        ctr.with_file(f"cmake-{machine}.sha256", client.host().file(f"{this_path}/cmake-{machine}.sha256"))
            .with_exec([
                "curl", "--fail", "-sSL", "--connect-timeout", "20", "--retry", "3", "--output", f"cmake-{machine}.sh",
                f"https://github.com/Kitware/CMake/releases/download/v3.27.6/cmake-3.27.6-linux-{machine}.sh",
            ])
            .with_exec(["sha256sum", "-c", f"cmake-{machine}.sha256"])
            .with_exec(["chmod", "u+x", f"./cmake-{machine}.sh"])
            .with_exec([f"./cmake-{machine}.sh", "--skip-license", "--prefix=/usr/local"])
    )

    return ctr


def build_centos_7(client, platform):
    ctr = client.container(platform=platform).from_("centos:7")

    pkgs = [pkg for pkg in _CENTOS_COMMON_PACKAGES] + ["bash"]

    ctr = (
        ctr.with_exec(["yum", "install", "-y", "epel-release"])
                .with_exec(["yum", "update", "-y"])
                .with_exec(["yum", "install", "-y"] + pkgs)
                .with_exec(["yum", "clean", "all"])
                .with_exec(["c_rehash"])
                .with_exec([
                    "mkdir", "-p",
                    "/root/rpmbuild/BUILD",
                    "/root/rpmbuild/RPMS",
                    "/root/rpmbuild/SOURCES",
                    "/root/rpmbuild/SPECS",
                    "/root/rpmbuild/SRPMS",
                ])
    )

    if platform == "linux/x86_64":
        machine = "x86_64"
    elif platform == "linux/arm64":
        machine = "aarch64"
    else:
        raise Exception("CentOS 7 supports only linux/amd64 and linux/arm64 platforms.")

    repo_path = str(Path(__file__).parent.parent.parent)
    this_path = os.path.join(repo_path, "packaging/dag")

    ctr = (
        ctr.with_file(f"cmake-{machine}.sha256", client.host().file(f"{this_path}/cmake-{machine}.sha256"))
            .with_exec([
                "curl", "--fail", "-sSL", "--connect-timeout", "20", "--retry", "3", "--output", f"cmake-{machine}.sh",
                f"https://github.com/Kitware/CMake/releases/download/v3.27.6/cmake-3.27.6-linux-{machine}.sh",
            ])
            .with_exec(["sha256sum", "-c", f"cmake-{machine}.sha256"])
            .with_exec(["chmod", "u+x", f"./cmake-{machine}.sh"])
            .with_exec([f"./cmake-{machine}.sh", "--skip-license", "--prefix=/usr/local"])
    )

    return ctr


_ROCKY_LINUX_COMMON_PACKAGES = [
    "autoconf",
    "autoconf-archive",
    "automake",
    "bash",
    "bison",
    "cmake",
    "cups-devel",
    "curl",
    "libcurl-devel",
    "diffutils",
    "elfutils-libelf-devel",
    "findutils",
    "flex",
    "freeipmi-devel",
    "gcc",
    "gcc-c++",
    "git",
    "golang",
    "json-c-devel",
    "libatomic",
    "libmnl-devel",
    "libtool",
    "libuuid-devel",
    "libuv-devel",
    "libyaml-devel",
    "libzstd-devel",
    "lm_sensors",
    "lz4-devel",
    "make",
    "ninja-build",
    "nc",
    "openssl-devel",
    "openssl-perl",
    "patch",
    "pcre2-devel",
    "pkgconfig",
    "pkgconfig(libmongoc-1.0)",
    "procps",
    "protobuf-c-devel",
    "protobuf-compiler",
    "protobuf-devel",
    "python3",
    "python3-pyyaml",
    "rpm-build",
    "rpm-devel",
    "rpmdevtools",
    "snappy-devel",
    "systemd-devel",
    "wget",
    "zlib-devel",
]


def build_rocky_linux_8(client, platform):
    ctr = client.container(platform=platform).from_("rockylinux:8")

    pkgs = [pkg for pkg in _ROCKY_LINUX_COMMON_PACKAGES] + ["autogen"]

    ctr = (
        ctr.with_exec(["dnf", "distro-sync", "-y", "--nodocs"])
                .with_exec(["dnf", "install", "-y", "--nodocs", "dnf-command(config-manager)", "epel-release"])
                .with_exec(["dnf", "config-manager", "--set-enabled", "powertools"])
                .with_exec(["dnf", "clean", "packages"])
                .with_exec([
                    "dnf", "install", "-y", "--nodocs", "--setopt=install_weak_deps=False", "--setopt=diskspacecheck=False",
                ] + pkgs)
                .with_exec(["rm", "-rf", "/var/cache/dnf"])
                .with_exec(["c_rehash"])
                .with_exec([
                    "mkdir", "-p",
                    "/root/rpmbuild/BUILD",
                    "/root/rpmbuild/RPMS",
                    "/root/rpmbuild/SOURCES",
                    "/root/rpmbuild/SPECS",
                    "/root/rpmbuild/SRPMS",
                ])
    )

    return ctr


def build_rocky_linux_9(client, platform):
    ctr = client.container(platform=platform).from_("rockylinux:9")

    pkgs = [pkg for pkg in _ROCKY_LINUX_COMMON_PACKAGES]

    ctr = (
        ctr.with_exec(["dnf", "distro-sync", "-y", "--nodocs"])
                .with_exec(["dnf", "install", "-y", "--nodocs", "dnf-command(config-manager)", "epel-release"])
                .with_exec(["dnf", "config-manager", "--set-enabled", "crb"])
                .with_exec(["dnf", "clean", "packages"])
                .with_exec([
                    "dnf", "install", "-y", "--allowerasing", "--nodocs", "--setopt=install_weak_deps=False", "--setopt=diskspacecheck=False",
                ] + pkgs)
                .with_exec(["rm", "-rf", "/var/cache/dnf"])
                .with_exec(["c_rehash"])
                .with_exec([
                    "mkdir", "-p",
                    "/root/rpmbuild/BUILD",
                    "/root/rpmbuild/RPMS",
                    "/root/rpmbuild/SOURCES",
                    "/root/rpmbuild/SPECS",
                    "/root/rpmbuild/SRPMS",
                ])
    )

    return ctr


_CENTOS_STREAM_COMMON_PACKAGES = [
    "autoconf",
    "autoconf-archive",
    "automake",
    "bash",
    "bison",
    "cmake",
    "cups-devel",
    "curl",
    "libcurl-devel",
    "libyaml-devel",
    "diffutils",
    "elfutils-libelf-devel",
    "findutils",
    "flex",
    "freeipmi-devel",
    "gcc",
    "gcc-c++",
    "git",
    "golang",
    "json-c-devel",
    "libatomic",
    "libmnl-devel",
    "libtool",
    "libuuid-devel",
    "libuv-devel",
    # "libzstd-devel",
    "lm_sensors",
    "lz4-devel",
    "make",
    "ninja-build",
    "nc",
    "openssl-devel",
    "openssl-perl",
    "patch",
    "pcre2-devel",
    "pkgconfig",
    "pkgconfig(libmongoc-1.0)",
    "procps",
    "protobuf-c-devel",
    "protobuf-compiler",
    "protobuf-devel",
    "python3",
    "python3-pyyaml",
    "rpm-build",
    "rpm-devel",
    "rpmdevtools",
    "snappy-devel",
    "systemd-devel",
    "wget",
    "zlib-devel",
]


def build_centos_stream_8(client, platform):
    ctr = client.container(platform=platform).from_("quay.io/centos/centos:stream8")

    pkgs = [pkg for pkg in _CENTOS_STREAM_COMMON_PACKAGES] + ["autogen"]

    ctr = (
        ctr.with_exec(["dnf", "distro-sync", "-y", "--nodocs"])
                .with_exec(["dnf", "install", "-y", "--nodocs", "dnf-command(config-manager)", "epel-release"])
                .with_exec(["dnf", "config-manager", "--set-enabled", "powertools"])
                .with_exec(["dnf", "clean", "packages"])
                .with_exec([
                    "dnf", "install", "-y", "--nodocs", "--setopt=install_weak_deps=False", "--setopt=diskspacecheck=False",
                ] + pkgs)
                .with_exec(["rm", "-rf", "/var/cache/dnf"])
                .with_exec(["c_rehash"])
                .with_exec([
                    "mkdir", "-p",
                    "/root/rpmbuild/BUILD",
                    "/root/rpmbuild/RPMS",
                    "/root/rpmbuild/SOURCES",
                    "/root/rpmbuild/SPECS",
                    "/root/rpmbuild/SRPMS",
                ])
    )

    return ctr


def build_centos_stream_9(client, platform):
    ctr = client.container(platform=platform).from_("quay.io/centos/centos:stream9")

    pkgs = [pkg for pkg in _CENTOS_STREAM_COMMON_PACKAGES]

    ctr = (
        ctr.with_exec(["dnf", "distro-sync", "-y", "--nodocs"])
                .with_exec(["dnf", "install", "-y", "--nodocs", "dnf-command(config-manager)", "epel-release"])
                .with_exec(["dnf", "config-manager", "--set-enabled", "crb"])
                .with_exec(["dnf", "clean", "packages"])
                .with_exec([
                    "dnf", "install", "-y", "--allowerasing", "--nodocs", "--setopt=install_weak_deps=False", "--setopt=diskspacecheck=False",
                ] + pkgs)
                .with_exec(["rm", "-rf", "/var/cache/dnf"])
                .with_exec(["c_rehash"])
                .with_exec([
                    "mkdir", "-p",
                    "/root/rpmbuild/BUILD",
                    "/root/rpmbuild/RPMS",
                    "/root/rpmbuild/SOURCES",
                    "/root/rpmbuild/SPECS",
                    "/root/rpmbuild/SRPMS",
                ])
    )

    return ctr


_ORACLE_LINUX_COMMON_PACKAGES = list(_ROCKY_LINUX_COMMON_PACKAGES)


def build_oracle_linux_9(client, platform):
    ctr = client.container(platform=platform).from_("oraclelinux:9")

    pkgs = [pkg for pkg in _ORACLE_LINUX_COMMON_PACKAGES]

    repo_path = str(Path(__file__).parent.parent.parent)
    this_path = os.path.join(repo_path, "packaging/dag")

    ctr = (
        ctr.with_file("/etc/yum.repos.d/ol9-epel.repo", client.host().file(f"{this_path}/ol9-epel.repo"))
                .with_exec(["dnf", "config-manager", "--set-enabled", "ol9_codeready_builder"])
                .with_exec(["dnf", "config-manager", "--set-enabled", "ol9_developer_EPEL"])
                .with_exec(["dnf", "distro-sync", "-y", "--nodocs"])
                .with_exec(["dnf", "clean", "-y", "packages"])
                .with_exec([
                    "dnf", "install", "-y", "--nodocs", "--setopt=install_weak_deps=False", "--setopt=diskspacecheck=False",
                ] + pkgs)
                .with_exec(["rm", "-rf", "/var/cache/dnf"])
                .with_exec(["c_rehash"])
                .with_exec([
                    "mkdir", "-p",
                    "/root/rpmbuild/BUILD",
                    "/root/rpmbuild/RPMS",
                    "/root/rpmbuild/SOURCES",
                    "/root/rpmbuild/SPECS",
                    "/root/rpmbuild/SRPMS",
                ])
    )

    return ctr


def build_oracle_linux_8(client, platform):
    ctr = client.container(platform=platform).from_("oraclelinux:8")

    pkgs = [pkg for pkg in _ORACLE_LINUX_COMMON_PACKAGES] + ["autogen"]

    repo_path = str(Path(__file__).parent.parent.parent)
    this_path = os.path.join(repo_path, "packaging/dag")

    ctr = (
        ctr.with_file("/etc/yum.repos.d/ol8-epel.repo", client.host().file(f"{this_path}/ol8-epel.repo"))
                .with_exec(["dnf", "config-manager", "--set-enabled", "ol8_codeready_builder"])
                .with_exec(["dnf", "distro-sync", "-y", "--nodocs"])
                .with_exec(["dnf", "clean", "-y", "packages"])
                .with_exec([
                    "dnf", "install", "-y", "--nodocs", "--setopt=install_weak_deps=False", "--setopt=diskspacecheck=False",
                ] + pkgs)
                .with_exec(["rm", "-rf", "/var/cache/dnf"])
                .with_exec(["c_rehash"])
                .with_exec([
                    "mkdir", "-p",
                    "/root/rpmbuild/BUILD",
                    "/root/rpmbuild/RPMS",
                    "/root/rpmbuild/SOURCES",
                    "/root/rpmbuild/SPECS",
                    "/root/rpmbuild/SRPMS",
                ])
    )

    return ctr

_OPENSUSE_COMMON_PACKAGES = [
    "autoconf",
    "autoconf-archive",
    "autogen",
    "automake",
    "bison",
    "cmake",
    "cups",
    "cups-devel",
    "curl",
    "diffutils",
    "flex",
    "freeipmi-devel",
    "gcc",
    "gcc-c++",
    "git-core",
    "go",
    "json-glib-devel",
    "judy-devel",
    "libatomic1",
    "libcurl-devel",
    "libelf-devel",
    "liblz4-devel",
    "libjson-c-devel",
    "libyaml-devel",
    "libmnl0",
    "libmnl-devel",
    "libnetfilter_acct1",
    "libnetfilter_acct-devel",
    "libpcre2-8-0",
    "libopenssl-devel",
    "libtool",
    "libuv-devel",
    "libuuid-devel",
    "libzstd-devel",
    "make",
    "ninja",
    "patch",
    "pkg-config",
    "protobuf-devel",
    "rpm-build",
    "rpm-devel",
    "rpmdevtools",
    "snappy-devel",
    "systemd-devel",
    "tar",
    "wget",
    "xen-devel",
]

def build_opensuse_tumbleweed(client, platform):
    ctr = client.container(platform=platform).from_("opensuse/tumbleweed:latest")

    pkgs = [pkg for pkg in _OPENSUSE_COMMON_PACKAGES] + ["protobuf-c"]

    ctr = (
        ctr.with_exec(["zypper", "update", "-y"])
                .with_exec([
                    "zypper", "install", "-y", "--allow-downgrade",
                ] + pkgs)
                .with_exec(["zypper", "clean"])
                .with_exec(["rm", "-rf", "/var/cache/zypp/*/*"])
                .with_exec(["c_rehash"])
                .with_exec([
                    "mkdir", "-p",
                    "/usr/src/packages/BUILD",
                    "/usr/src/packages/RPMS",
                    "/usr/src/packages/SOURCES",
                    "/usr/src/packages/SPECS",
                    "/usr/src/packages/SRPMS",
                ])
    )

    return ctr


def build_opensuse_15_5(client, platform):
    ctr = client.container(platform=platform).from_("opensuse/leap:15.5")

    pkgs = [pkg for pkg in _OPENSUSE_COMMON_PACKAGES] + ["libprotobuf-c-devel"]

    ctr = (
        ctr.with_exec(["zypper", "update", "-y"])
                .with_exec([
                    "zypper", "install", "-y", "--allow-downgrade",
                ] + pkgs)
                .with_exec(["zypper", "clean"])
                .with_exec(["rm", "-rf", "/var/cache/zypp/*/*"])
                .with_exec(["c_rehash"])
                .with_exec([
                    "mkdir", "-p",
                    "/usr/src/packages/BUILD",
                    "/usr/src/packages/RPMS",
                    "/usr/src/packages/SOURCES",
                    "/usr/src/packages/SPECS",
                    "/usr/src/packages/SRPMS",
                ])
    )

    return ctr


def build_opensuse_15_4(client, platform):
    crt = client.container(platform=platform).from_("opensuse/leap:15.4")

    pkgs = [pkg for pkg in _OPENSUSE_COMMON_PACKAGES] + ["libprotobuf-c-devel"]

    crt = (
        crt.with_exec(["zypper", "update", "-y"])
                .with_exec([
                    "zypper", "install", "-y", "--allow-downgrade",
                ] + pkgs)
                .with_exec(["zypper", "clean"])
                .with_exec(["rm", "-rf", "/var/cache/zypp/*/*"])
                .with_exec(["c_rehash"])
                .with_exec([
                    "mkdir", "-p",
                    "/usr/src/packages/BUILD",
                    "/usr/src/packages/RPMS",
                    "/usr/src/packages/SOURCES",
                    "/usr/src/packages/SPECS",
                    "/usr/src/packages/SRPMS",
                ])
    )

    return crt


_FEDORA_COMMON_PACKAGES = [
    "autoconf",
    "autoconf-archive",
    "autogen",
    "automake",
    "bash",
    "bison",
    "cmake",
    "cups-devel",
    "curl",
    "diffutils",
    "elfutils-libelf-devel",
    "findutils",
    "flex",
    "freeipmi-devel",
    "gcc",
    "gcc-c++",
    "git-core",
    "golang",
    "json-c-devel",
    "libcurl-devel",
    "libyaml-devel",
    "Judy-devel",
    "libatomic",
    "libmnl-devel",
    "libnetfilter_acct-devel",
    "libtool",
    "libuuid-devel",
    "libuv-devel",
    "libzstd-devel",
    "lz4-devel",
    "make",
    "ninja-build",
    "openssl-devel",
    "openssl-perl",
    "patch",
    "pcre2-devel",
    "pkgconfig",
]


def build_fedora_37(client, platform):
    ctr = client.container(platform=platform).from_("fedora:37")

    pkgs = [pkg for pkg in _FEDORA_COMMON_PACKAGES]

    ctr = (
        ctr.with_exec(["dnf", "distro-sync", "-y", "--nodocs"])
                .with_exec(["dnf", "clean", "-y", "packages"])
                .with_exec([
                    "dnf", "install", "-y", "--nodocs", "--setopt=install_weak_deps=False", "--setopt=diskspacecheck=False",
                ] + pkgs)
                .with_exec(["rm", "-rf", "/var/cache/dnf"])
                .with_exec(["c_rehash"])
                .with_exec([
                    "mkdir", "-p",
                    "/root/rpmbuild/BUILD",
                    "/root/rpmbuild/RPMS",
                    "/root/rpmbuild/SOURCES",
                    "/root/rpmbuild/SPECS",
                    "/root/rpmbuild/SRPMS",
                ])
    )

    return ctr


def build_fedora_38(client, platform):
    ctr = client.container(platform=platform).from_("fedora:38")

    pkgs = [pkg for pkg in _FEDORA_COMMON_PACKAGES]

    ctr = (
        ctr.with_exec(["dnf", "distro-sync", "-y", "--nodocs"])
                .with_exec(["dnf", "clean", "-y", "packages"])
                .with_exec([
                    "dnf", "install", "-y", "--nodocs", "--setopt=install_weak_deps=False", "--setopt=diskspacecheck=False",
                ] + pkgs)
                .with_exec(["rm", "-rf", "/var/cache/dnf"])
                .with_exec(["c_rehash"])
                .with_exec([
                    "mkdir", "-p",
                    "/root/rpmbuild/BUILD",
                    "/root/rpmbuild/RPMS",
                    "/root/rpmbuild/SOURCES",
                    "/root/rpmbuild/SPECS",
                    "/root/rpmbuild/SRPMS",
                ])
    )

    return ctr


def build_fedora_39(client, platform):
    ctr = client.container(platform=platform).from_("fedora:39")

    pkgs = [pkg for pkg in _FEDORA_COMMON_PACKAGES]

    ctr = (
        ctr.with_exec(["dnf", "distro-sync", "-y", "--nodocs"])
                .with_exec(["dnf", "clean", "-y", "packages"])
                .with_exec([
                    "dnf", "install", "-y", "--nodocs", "--setopt=install_weak_deps=False", "--setopt=diskspacecheck=False",
                ] + pkgs)
                .with_exec(["rm", "-rf", "/var/cache/dnf"])
                .with_exec(["c_rehash"])
                .with_exec([
                    "mkdir", "-p",
                    "/root/rpmbuild/BUILD",
                    "/root/rpmbuild/RPMS",
                    "/root/rpmbuild/SOURCES",
                    "/root/rpmbuild/SPECS",
                    "/root/rpmbuild/SRPMS",
                ])
    )

    return ctr


_DEBIAN_COMMON_PACKAGES = [
    "autoconf",
    "autoconf-archive",
    "autogen",
    "automake",
    "bison",
    "build-essential",
    "ca-certificates",
    "cmake",
    "curl",
    "dh-autoreconf",
    "dh-make",
    "dpkg-dev",
    "flex",
    "g++",
    "gcc",
    "git-buildpackage",
    "git-core",
    "golang",
    "libatomic1",
    "libcurl4-openssl-dev",
    "libcups2-dev",
    "libdistro-info-perl",
    "libelf-dev",
    "libipmimonitoring-dev",
    "libjson-c-dev",
    "libyaml-dev",
    "libjudy-dev",
    "liblz4-dev",
    "libmnl-dev",
    "libmongoc-dev",
    "libnetfilter-acct-dev",
    "libpcre2-dev",
    "libprotobuf-dev",
    "libprotoc-dev",
    "libsnappy-dev",
    "libsystemd-dev",
    "libssl-dev",
    "libtool",
    "libuv1-dev",
    "libzstd-dev",
    "make",
    "ninja-build",
    "pkg-config",
    "protobuf-compiler",
    "systemd",
    "uuid-dev",
    "wget",
    "zlib1g-dev",
]


def build_debian_10(client, platform):
    ctr = client.container(platform=platform).from_("debian:buster")

    pkgs = [pkg for pkg in _DEBIAN_COMMON_PACKAGES] + ["dh-systemd", "libxen-dev"]

    ctr = (
        ctr.with_env_variable("DEBIAN_FRONTEND", "noninteractive")
                .with_exec(["apt-get", "update"])
                .with_exec(["apt-get", "upgrade", "-y"])
                .with_exec(["apt-get", "install", "-y", "--no-install-recommends"] + pkgs)
                .with_exec(["apt-get", "clean"])
                .with_exec(["c_rehash"])
                .with_exec(["rm", "-rf", "/var/lib/apt/lists/*"])
    )

    return ctr


def build_debian_11(client, platform):
    ctr = client.container(platform=platform).from_("debian:bullseye")

    pkgs = [pkg for pkg in _DEBIAN_COMMON_PACKAGES] + ["libxen-dev"]

    ctr = (
        ctr.with_env_variable("DEBIAN_FRONTEND", "noninteractive")
                .with_exec(["apt-get", "update"])
                .with_exec(["apt-get", "upgrade", "-y"])
                .with_exec(["apt-get", "install", "-y", "--no-install-recommends"] + pkgs)
                .with_exec(["apt-get", "clean"])
                .with_exec(["c_rehash"])
                .with_exec(["rm", "-rf", "/var/lib/apt/lists/*"])
    )

    return ctr

def build_debian_12(client, platform) -> dagger.Container:
    ctr = client.container(platform=platform).from_("debian:bookworm")

    pkgs = [pkg for pkg in _DEBIAN_COMMON_PACKAGES]

    if platform != dagger.Platform("linux/i386"):
        pkgs.append("libxen-dev")

    ctr = (
        ctr.with_env_variable("DEBIAN_FRONTEND", "noninteractive")
                .with_exec(["apt-get", "update"])
                .with_exec(["apt-get", "upgrade", "-y"])
                .with_exec(["apt-get", "install", "-y", "--no-install-recommends"] + pkgs)
                .with_exec(["apt-get", "clean"])
                .with_exec(["c_rehash"])
                .with_exec(["rm", "-rf", "/var/lib/apt/lists/*"])
    )

    return ctr


_UBUNTU_COMMON_PACKAGES = [
    "autoconf",
    "autoconf-archive",
    "autogen",
    "automake",
    "bison",
    "build-essential",
    "ca-certificates",
    "cmake",
    "curl",
    "dh-autoreconf",
    "dh-make",
    "dpkg-dev",
    "flex",
    "g++",
    "gcc",
    "git-buildpackage",
    "git-core",
    "golang",
    "libatomic1",
    "libcurl4-openssl-dev",
    "libcups2-dev",
    "libdistro-info-perl",
    "libelf-dev",
    "libipmimonitoring-dev",
    "libjson-c-dev",
    "libyaml-dev",
    "libjudy-dev",
    "liblz4-dev",
    "libmnl-dev",
    "libmongoc-dev",
    "libnetfilter-acct-dev",
    "libpcre2-dev",
    "libprotobuf-dev",
    "libprotoc-dev",
    "libsnappy-dev",
    "libsystemd-dev",
    "libssl-dev",
    "libtool",
    "libuv1-dev",
    "libxen-dev",
    "libzstd-dev",
    "make",
    "ninja-build",
    "pkg-config",
    "protobuf-compiler",
    "systemd",
    "uuid-dev",
    "wget",
    "zlib1g-dev",
]

def build_ubuntu_20_04(client, platform):
    ctr = client.container(platform=platform).from_("ubuntu:20.04")

    pkgs = [pkg for pkg in _UBUNTU_COMMON_PACKAGES] + ["dh-systemd"]

    ctr = (
        ctr.with_env_variable("DEBIAN_FRONTEND", "noninteractive")
                .with_exec(["apt-get", "update"])
                .with_exec(["apt-get", "upgrade", "-y"])
                .with_exec(["apt-get", "install", "-y", "--no-install-recommends"] + pkgs)
                .with_exec(["apt-get", "clean"])
                .with_exec(["c_rehash"])
                .with_exec(["rm", "-rf", "/var/lib/apt/lists/*"])
    )

    #
    # FIXME: add kitware for cmake on arm-hf
    #

    return ctr


def build_ubuntu_22_04(client, platform):
    ctr = client.container(platform=platform).from_("ubuntu:22.04")

    pkgs = [pkg for pkg in _UBUNTU_COMMON_PACKAGES]

    ctr = (
        ctr.with_env_variable("DEBIAN_FRONTEND", "noninteractive")
                .with_exec(["apt-get", "update"])
                .with_exec(["apt-get", "upgrade", "-y"])
                .with_exec(["apt-get", "install", "-y", "--no-install-recommends"] + pkgs)
                .with_exec(["apt-get", "clean"])
                .with_exec(["c_rehash"])
                .with_exec(["rm", "-rf", "/var/lib/apt/lists/*"])
    )

    return ctr


def build_ubuntu_23_04(client, platform):
    ctr = client.container(platform=platform).from_("ubuntu:23.04")

    pkgs = [pkg for pkg in _UBUNTU_COMMON_PACKAGES]

    ctr = (
        ctr.with_env_variable("DEBIAN_FRONTEND", "noninteractive")
                .with_exec(["apt-get", "update"])
                .with_exec(["apt-get", "upgrade", "-y"])
                .with_exec(["apt-get", "install", "-y", "--no-install-recommends"] + pkgs)
                .with_exec(["apt-get", "clean"])
                .with_exec(["c_rehash"])
                .with_exec(["rm", "-rf", "/var/lib/apt/lists/*"])
    )

    return ctr


def build_ubuntu_23_10(client, platform):
    ctr = client.container(platform=platform).from_("ubuntu:23.10")

    pkgs = [pkg for pkg in _UBUNTU_COMMON_PACKAGES]

    ctr = (
        ctr.with_env_variable("DEBIAN_FRONTEND", "noninteractive")
                .with_exec(["apt-get", "update"])
                .with_exec(["apt-get", "upgrade", "-y"])
                .with_exec(["apt-get", "install", "-y", "--no-install-recommends"] + pkgs)
                .with_exec(["apt-get", "clean"])
                .with_exec(["c_rehash"])
                .with_exec(["rm", "-rf", "/var/lib/apt/lists/*"])
    )

    return ctr


class Image:
    def __init__(self, cli_name, docker_name, deps_fn, supported_platforms=None):
        self.cli_name = cli_name
        self.docker_name = docker_name
        self.supported_platforms = supported_platforms
        self.deps_fn = deps_fn

    def __hash__(self):
        return hash(self.cli_name)

    def __eq__(self, other):
        return isinstance(other, Image) and self.cli_name == other.cli_name

    def __lt__(self, other):
        return self.cli_name < other.cli_name

    def __str__(self):
        return self.cli_name

    def build(self, client, platform):
        ctr = self.deps_fn(client, platform)

        ctr = (
            ctr.with_workdir("/")
               .with_exec(["sh", "-c", "curl https://sh.rustup.rs -sSf | sh -s -- -y"])
        )

        bin_paths = [
            "/root/.cargo/bin",
            "/usr/local/sbin",
            "/usr/local/bin",
            "/usr/sbin",
            "/usr/bin",
            "/sbin",
            "/bin",
        ]

        ctr = ctr.with_env_variable("PATH", ":".join(bin_paths))

        ctr = (
            ctr.with_exec(["cargo", "new", "--bin", "hello"])
               .with_workdir("/hello")
               .with_exec(["cargo", "run", "-v", "-v"])
        )

        return ctr


SUPPORTED_IMAGES = {
    Image("alpine_3_18", "alpine:3.18", build_alpine_3_18),
    Image("alpine_3_19", "alpine:3.19", build_alpine_3_19),
    Image("amazonlinux2", "amazonlinux:2", build_amazon_linux_2),
    # amazonlinux2023
    Image("centos7", "centos:7", build_centos_7),
    Image("centos-stream8", "quay.io/centos/centos:stream8", build_centos_stream_8),
    Image("centos-stream9", "quay.io/centos/centos:stream9", build_centos_stream_9),
    Image("debian10", "debian:buster", build_debian_10),
    Image("debian11", "debian:bullseye", build_debian_11),
    Image("debian12", "debian:bookworm", build_debian_12),
    Image("fedora37", "fedora:37", build_fedora_37),
    Image("fedora38", "fedora:38", build_fedora_38),
    Image("fedora39", "fedora:39", build_fedora_39),
    Image("opensuse15.4", "opensuse/leap:15.4", build_opensuse_15_4),
    Image("opensuse15.5", "opensuse/leap:15.5", build_opensuse_15_5),
    Image("opensusetumbleweed", "opensuse/tumbleweed:latest", build_opensuse_tumbleweed),
    Image("oraclelinux8", "oraclelinux:8", build_oracle_linux_8),
    Image("oraclelinux9", "oraclelinux:9", build_oracle_linux_9),
    Image("rockylinux8", "rockylinux:8", build_rocky_linux_8),
    Image("rockylinux9", "rockylinux:9", build_rocky_linux_9),
    Image("ubuntu20.04", "ubuntu:20.04", build_ubuntu_20_04),
    Image("ubuntu22.04", "ubuntu:22.04", build_ubuntu_22_04),
    Image("ubuntu23.04", "ubuntu:23.04", build_ubuntu_23_04),
    Image("ubuntu23.10", "ubuntu:23.10", build_ubuntu_23_10),
}
