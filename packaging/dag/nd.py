from typing import List

import enum
import os
import pathlib
import uuid

import dagger
import jinja2

import imageutils


class Platform:
    def __init__(self, platform: str):
        self.platform = dagger.Platform(platform)

    def escaped(self) -> str:
        return str(self.platform).removeprefix("linux/").replace("/", "_")

    def __eq__(self, other):
        if isinstance(other, Platform):
            return self.platform == other.platform
        elif isinstance(other, dagger.Platform):
            return self.platform == other
        else:
            return NotImplemented

    def __ne__(self, other):
        return not (self == other)

    def __hash__(self):
        return hash(self.platform)

    def __str__(self) -> str:
        return str(self.platform)


SUPPORTED_PLATFORMS = set(
    [
        Platform("linux/x86_64"),
        Platform("linux/arm64"),
        Platform("linux/i386"),
        Platform("linux/arm/v7"),
        Platform("linux/arm/v6"),
        Platform("linux/ppc64le"),
        Platform("linux/s390x"),
        Platform("linux/riscv64"),
    ]
)


SUPPORTED_DISTRIBUTIONS = set(
    [
        "alpine_3_18",
        "alpine_3_19",
        "amazonlinux2",
        "centos7",
        "centos-stream8",
        "centos-stream9",
        "debian10",
        "debian11",
        "debian12",
        "fedora37",
        "fedora38",
        "fedora39",
        "opensuse15.4",
        "opensuse15.5",
        "opensusetumbleweed",
        "oraclelinux8",
        "oraclelinux9",
        "rockylinux8",
        "rockylinux9",
        "ubuntu20.04",
        "ubuntu22.04",
        "ubuntu23.04",
        "ubuntu23.10",
    ]
)


class Distribution:
    def __init__(self, display_name):
        self.display_name = display_name

        if self.display_name == "alpine_3_18":
            self.docker_tag = "alpine:3.18"
            self.builder = imageutils.build_alpine_3_18
            self.platforms = SUPPORTED_PLATFORMS
        elif self.display_name == "alpine_3_19":
            self.docker_tag = "alpine:3.19"
            self.builder = imageutils.build_alpine_3_19
            self.platforms = SUPPORTED_PLATFORMS
        elif self.display_name == "amazonlinux2":
            self.docker_tag = "amazonlinux:2"
            self.builder = imageutils.build_amazon_linux_2
            self.platforms = SUPPORTED_PLATFORMS
        elif self.display_name == "centos7":
            self.docker_tag = "centos:7"
            self.builder = imageutils.build_centos_7
            self.platforms = SUPPORTED_PLATFORMS
        elif self.display_name == "centos-stream8":
            self.docker_tag = "quay.io/centos/centos:stream8"
            self.builder = imageutils.build_centos_stream_8
            self.platforms = SUPPORTED_PLATFORMS
        elif self.display_name == "centos-stream9":
            self.docker_tag = "quay.io/centos/centos:stream9"
            self.builder = imageutils.build_centos_stream_9
            self.platforms = SUPPORTED_PLATFORMS
        elif self.display_name == "debian10":
            self.docker_tag = "debian:10"
            self.builder = imageutils.build_debian_10
            self.platforms = SUPPORTED_PLATFORMS
        elif self.display_name == "debian11":
            self.docker_tag = "debian:11"
            self.builder = imageutils.build_debian_11
            self.platforms = SUPPORTED_PLATFORMS
        elif self.display_name == "debian12":
            self.docker_tag = "debian:12"
            self.builder = imageutils.build_debian_12
            self.platforms = SUPPORTED_PLATFORMS
        elif self.display_name == "fedora37":
            self.docker_tag = "fedora:37"
            self.builder = imageutils.build_fedora_37
            self.platforms = SUPPORTED_PLATFORMS
        elif self.display_name == "fedora38":
            self.docker_tag = "fedora:38"
            self.builder = imageutils.build_fedora_38
            self.platforms = SUPPORTED_PLATFORMS
        elif self.display_name == "fedora39":
            self.docker_tag = "fedora:39"
            self.platforms = SUPPORTED_PLATFORMS
            self.builder = imageutils.build_fedora_39
        elif self.display_name == "opensuse15.4":
            self.docker_tag = "opensuse/leap:15.4"
            self.builder = imageutils.build_opensuse_15_4
            self.platforms = SUPPORTED_PLATFORMS
        elif self.display_name == "opensuse15.5":
            self.docker_tag = "opensuse/leap:15.5"
            self.builder = imageutils.build_opensuse_15_5
            self.platforms = SUPPORTED_PLATFORMS
        elif self.display_name == "opensusetumbleweed":
            self.docker_tag = "opensuse/tumbleweed:latest"
            self.builder = imageutils.build_opensuse_tumbleweed
            self.platforms = SUPPORTED_PLATFORMS
        elif self.display_name == "oraclelinux8":
            self.docker_tag = "oraclelinux:8"
            self.builder = imageutils.build_oracle_linux_8
            self.platforms = SUPPORTED_PLATFORMS
        elif self.display_name == "oraclelinux9":
            self.docker_tag = "oraclelinux:9"
            self.builder = imageutils.build_oracle_linux_9
            self.platforms = SUPPORTED_PLATFORMS
        elif self.display_name == "rockylinux8":
            self.docker_tag = "rockylinux:8"
            self.builder = imageutils.build_rocky_linux_8
            self.platforms = SUPPORTED_PLATFORMS
        elif self.display_name == "rockylinux9":
            self.docker_tag = "rockylinux:9"
            self.builder = imageutils.build_rocky_linux_9
            self.platforms = SUPPORTED_PLATFORMS
        elif self.display_name == "ubuntu20.04":
            self.docker_tag = "ubuntu:20.04"
            self.builder = imageutils.build_ubuntu_20_04
            self.platforms = SUPPORTED_PLATFORMS
        elif self.display_name == "ubuntu22.04":
            self.docker_tag = "ubuntu:22.04"
            self.builder = imageutils.build_ubuntu_22_04
            self.platforms = SUPPORTED_PLATFORMS
        elif self.display_name == "ubuntu23.04":
            self.docker_tag = "ubuntu:23.04"
            self.builder = imageutils.build_ubuntu_23_04
            self.platforms = SUPPORTED_PLATFORMS
        elif self.display_name == "ubuntu23.10":
            self.docker_tag = "ubuntu:23.10"
            self.builder = imageutils.build_ubuntu_23_10
            self.platforms = SUPPORTED_PLATFORMS
        else:
            raise ValueError(f"Unknown distribution: {self.display_name}")

    def _cache_volume(
        self, client: dagger.Client, platform: dagger.Platform, path: str
    ) -> dagger.CacheVolume:
        tag = "_".join([self.display_name, Platform(platform).escaped()])
        return client.cache_volume(f"{path}-{tag}")

    def build(
        self, client: dagger.Client, platform: dagger.Platform
    ) -> dagger.Container:
        if platform not in self.platforms:
            raise ValueError(
                f"Building {self.display_name} is not supported on {platform}."
            )

        ctr = self.builder(client, platform)
        ctr = imageutils.install_cargo(ctr)

        return ctr


class FeatureFlags(enum.Flag):
    DBEngine = enum.auto()
    GoPlugin = enum.auto()
    ExtendedBPF = enum.auto()
    LogsManagement = enum.auto()
    MachineLearning = enum.auto()
    BundledProtobuf = enum.auto()


class NetdataInstaller:
    def __init__(
        self,
        platform: Platform,
        distro: Distribution,
        repo_root: pathlib.Path,
        prefix: pathlib.Path,
        features: FeatureFlags,
    ):
        self.platform = platform
        self.distro = distro
        self.repo_root = repo_root
        self.prefix = prefix
        self.features = features

    def _mount_repo(
        self, client: dagger.Client, ctr: dagger.Container, repo_root: pathlib.Path
    ) -> dagger.Container:
        host_repo_root = pathlib.Path(__file__).parent.parent.parent.as_posix()
        exclude_dirs = ["build", "fluent-bit/build", "packaging/dag"]

        # The installer builds/stores intermediate artifacts under externaldeps/
        # We add a volume to speed up rebuilds. The volume has to be unique
        # per platform/distro in order to avoid mixing unrelated artifacts
        # together.
        externaldeps = self.distro._cache_volume(client, self.platform, "externaldeps")

        ctr = (
            ctr.with_directory(
                self.repo_root.as_posix(), client.host().directory(host_repo_root)
            )
            .with_workdir(self.repo_root.as_posix())
            .with_mounted_cache(
                os.path.join(self.repo_root, "externaldeps"), externaldeps
            )
        )

        return ctr

    def install(self, client: dagger.Client, ctr: dagger.Container) -> dagger.Container:
        args = ["--dont-wait", "--dont-start-it", "--disable-telemetry"]

        if FeatureFlags.DBEngine not in self.features:
            args.append("--disable-dbengine")

        if FeatureFlags.GoPlugin not in self.features:
            args.append("--disable-go")

        if FeatureFlags.ExtendedBPF not in self.features:
            args.append("--disable-ebpf")

        if FeatureFlags.MachineLearning not in self.features:
            args.append("--disable-ml")

        if FeatureFlags.BundledProtobuf not in self.features:
            args.append("--use-system-protobuf")

        args.extend(["--install-prefix", self.prefix.parent.as_posix()])

        ctr = self._mount_repo(client, ctr, self.repo_root.as_posix())

        ctr = ctr.with_env_variable(
            "NETDATA_CMAKE_OPTIONS", "-DCMAKE_BUILD_TYPE=Debug"
        ).with_exec(["./netdata-installer.sh"] + args)

        return ctr


class Endpoint:
    def __init__(self, hostname: str, port: int):
        self.hostname = hostname
        self.port = port

    def __str__(self):
        return ":".join([self.hostname, str(self.port)])


class ChildStreamConf:
    def __init__(
        self,
        installer: NetdataInstaller,
        destinations: List[Endpoint],
        api_key: uuid.UUID,
    ):
        self.installer = installer
        self.substitutions = {
            "enabled": "yes",
            "destination": " ".join([str(dst) for dst in destinations]),
            "api_key": api_key,
            "timeout_seconds": 60,
            "default_port": 19999,
            "send_charts_matching": "*",
            "buffer_size_bytes": 1024 * 1024,
            "reconnect_delay_seconds": 5,
            "initial_clock_resync_iterations": 60,
        }

    def render(self) -> str:
        tmpl_path = pathlib.Path(__file__).parent / "files/child_stream.conf"
        with open(tmpl_path) as fp:
            tmpl = jinja2.Template(fp.read())

        return tmpl.render(**self.substitutions)


class ParentStreamConf:
    def __init__(self, installer: NetdataInstaller, api_key: uuid.UUID):
        self.installer = installer
        self.substitutions = {
            "api_key": str(api_key),
            "enabled": "yes",
            "allow_from": "*",
            "default_history": 3600,
            "health_enabled_by_default": "auto",
            "default_postpone_alarms_on_connect_seconds": 60,
            "multiple_connections": "allow",
        }

    def render(self) -> str:
        tmpl_path = pathlib.Path(__file__).parent / "files/parent_stream.conf"
        with open(tmpl_path) as fp:
            tmpl = jinja2.Template(fp.read())

        return tmpl.render(**self.substitutions)


class StreamConf:
    def __init__(self, child_conf: ChildStreamConf, parent_conf: ParentStreamConf):
        self.child_conf = child_conf
        self.parent_conf = parent_conf

    def render(self) -> str:
        child_section = self.child_conf.render() if self.child_conf else ""
        parent_section = self.parent_conf.render() if self.parent_conf else ""
        return "\n".join([child_section, parent_section])


class AgentContext:
    def __init__(
        self,
        client: dagger.Client,
        platform: dagger.Platform,
        distro: Distribution,
        installer: NetdataInstaller,
        endpoint: Endpoint,
        api_key: uuid.UUID,
        allow_children: bool,
    ):
        self.client = client
        self.platform = platform
        self.distro = distro
        self.installer = installer
        self.endpoint = endpoint
        self.api_key = api_key
        self.allow_children = allow_children

        self.parent_contexts = []

        self.built_distro = False
        self.built_agent = False

    def add_parent(self, parent_context: "AgentContext"):
        self.parent_contexts.append(parent_context)

    def build_container(self) -> dagger.Container:
        ctr = self.distro.build(self.client, self.platform)
        ctr = self.installer.install(self.client, ctr)

        if len(self.parent_contexts) == 0 and not self.allow_children:
            return ctr.with_exposed_port(self.endpoint.port)

        destinations = [parent_ctx.endpoint for parent_ctx in self.parent_contexts]
        child_stream_conf = ChildStreamConf(self.installer, destinations, self.api_key)

        parent_stream_conf = None
        if self.allow_children:
            parent_stream_conf = ParentStreamConf(self.installer, self.api_key)

        stream_conf = StreamConf(child_stream_conf, parent_stream_conf)

        # write the stream conf to localhost and cp it in the container
        host_stream_conf_path = pathlib.Path(
            f"/tmp/{self.endpoint.hostname}_stream.conf"
        )
        with open(host_stream_conf_path, "w") as fp:
            fp.write(stream_conf.render())

        ctr_stream_conf_path = self.installer.prefix / "etc/netdata/stream.conf"

        ctr = ctr.with_file(
            ctr_stream_conf_path.as_posix(),
            self.client.host().file(host_stream_conf_path.as_posix()),
        )

        ctr = ctr.with_exposed_port(self.endpoint.port)

        return ctr
