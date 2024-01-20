#!/usr/bin/env python3

from typing import Callable, List, Tuple

import asyncio
import enum
import os
import pathlib
import sys
import tempfile
import time
import uuid

import anyio
import click
import dagger
import jinja2

import images as oci_images


class Platform:
    def __init__(self, platform: str):
        self.platform = dagger.Platform(platform)

    def escaped(self) -> str:
        return str(self.platform).removeprefix("linux/").replace('/', '_')

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


SUPPORTED_PLATFORMS = set([
    Platform("linux/x86_64"),
    Platform("linux/arm64"),
    Platform("linux/i386"),
    Platform("linux/arm/v7"),
    Platform("linux/arm/v6"),
    Platform("linux/ppc64le"),
    Platform("linux/s390x"),
    Platform("linux/riscv64"),
])


class Distribution:
    def __init__(self, display_name):
        self.display_name = display_name

        if self.display_name == "alpine_3_18":
            self.docker_tag = "alpine:3.18"
            self.builder = oci_images.build_alpine_3_18
            self.platforms = SUPPORTED_PLATFORMS
        elif self.display_name == "alpine_3_19":
            self.docker_tag = "alpine:3.19"
            self.builder = oci_images.build_alpine_3_19
            self.platforms = SUPPORTED_PLATFORMS
        elif self.display_name == "amazonlinux2":
            self.docker_tag = "amazonlinux:2"
            self.builder = oci_images.build_amazon_linux_2
            self.platforms = SUPPORTED_PLATFORMS
        elif self.display_name == "centos7":
            self.docker_tag = "centos:7"
            self.builder = oci_images.build_centos_7
            self.platforms = SUPPORTED_PLATFORMS
        elif self.display_name == "centos-stream8":
            self.docker_tag = "quay.io/centos/centos:stream8"
            self.builder = oci_images.build_centos_stream_8
            self.platforms = SUPPORTED_PLATFORMS
        elif self.display_name == "centos-stream9":
            self.docker_tag = "quay.io/centos/centos:stream9"
            self.builder = oci_images.build_centos_stream_9
            self.platforms = SUPPORTED_PLATFORMS
        elif self.display_name == "debian10":
            self.docker_tag = "debian:10"
            self.builder = oci_images.build_debian_10
            self.platforms = SUPPORTED_PLATFORMS
        elif self.display_name == "debian11":
            self.docker_tag = "debian:11"
            self.builder = oci_images.build_debian_11
            self.platforms = SUPPORTED_PLATFORMS
        elif self.display_name == "debian12":
            self.docker_tag = "debian:12"
            self.builder = oci_images.build_debian_12
            self.platforms = SUPPORTED_PLATFORMS
        elif self.display_name == "fedora37":
            self.docker_tag = "fedora:37"
            self.builder = oci_images.build_fedora_37
            self.platforms = SUPPORTED_PLATFORMS
        elif self.display_name == "fedora38":
            self.docker_tag = "fedora:38"
            self.builder = oci_images.build_fedora_38
            self.platforms = SUPPORTED_PLATFORMS
        elif self.display_name == "fedora39":
            self.docker_tag = "fedora:39"
            self.platforms = SUPPORTED_PLATFORMS
            self.builder = oci_images.build_fedora_39
        elif self.display_name == "opensuse15.4":
            self.docker_tag = "opensuse/leap:15.4"
            self.builder = oci_images.build_opensuse_15_4
            self.platforms = SUPPORTED_PLATFORMS
        elif self.display_name == "opensuse15.5":
            self.docker_tag = "opensuse/leap:15.5"
            self.builder = oci_images.build_opensuse_15_5
            self.platforms = SUPPORTED_PLATFORMS
        elif self.display_name == "opensusetumbleweed":
            self.docker_tag = "opensuse/tumbleweed:latest"
            self.builder = oci_images.build_opensuse_tumbleweed
            self.platforms = SUPPORTED_PLATFORMS
        elif self.display_name == "oraclelinux8":
            self.docker_tag = "oraclelinux:8"
            self.builder = oci_images.build_oracle_linux_8
            self.platforms = SUPPORTED_PLATFORMS
        elif self.display_name == "oraclelinux9":
            self.docker_tag = "oraclelinux:9"
            self.builder = oci_images.build_oracle_linux_9
            self.platforms = SUPPORTED_PLATFORMS
        elif self.display_name == "rockylinux8":
            self.docker_tag = "rockylinux:8"
            self.builder = oci_images.build_rocky_linux_8
            self.platforms = SUPPORTED_PLATFORMS
        elif self.display_name == "rockylinux9":
            self.docker_tag = "rockylinux:9"
            self.builder = oci_images.build_rocky_linux_9
            self.platforms = SUPPORTED_PLATFORMS
        elif self.display_name == "ubuntu20.04":
            self.docker_tag = "ubuntu:20.04"
            self.builder = oci_images.build_ubuntu_20_04
            self.platforms = SUPPORTED_PLATFORMS
        elif self.display_name == "ubuntu22.04":
            self.docker_tag = "ubuntu:22.04"
            self.builder = oci_images.build_ubuntu_22_04
            self.platforms = SUPPORTED_PLATFORMS
        elif self.display_name == "ubuntu23.04":
            self.docker_tag = "ubuntu:23.04"
            self.builder = oci_images.build_ubuntu_23_04
            self.platforms = SUPPORTED_PLATFORMS
        elif self.display_name == "ubuntu23.10":
            self.docker_tag = "ubuntu:23.10"
            self.builder = oci_images.build_ubuntu_23_10
            self.platforms = SUPPORTED_PLATFORMS
        else:
            raise ValueError(f"Unknown distribution: {self.display_name}")


    def _cache_volume(self, client: dagger.Client, platform: dagger.Platform, path: str) -> dagger.CacheVolume:
        tag = "_".join([self.display_name, Platform(platform).escaped()])
        return client.cache_volume(f"{path}-{tag}")


    def build(self, client: dagger.Client, platform: dagger.Platform) -> dagger.Container:
        if platform not in self.platforms:
            raise ValueError(f"Building {self.display_name} is not supported on {platform}.")

        ctr = self.builder(client, platform)
        ctr = oci_images.install_cargo(ctr)

        return ctr


class FeatureFlags(enum.Flag):
    DBEngine = enum.auto()
    GoPlugin = enum.auto()
    ExtendedBPF = enum.auto()
    LogsManagement = enum.auto()
    MachineLearning = enum.auto()
    BundledProtobuf = enum.auto()


class NetdataInstaller:
    def __init__(self,
                 platform: Platform,
                 distro: Distribution,
                 repo_root: pathlib.Path,
                 prefix: pathlib.Path,
                 features: FeatureFlags):
        self.platform = platform
        self.distro = distro
        self.repo_root = repo_root
        self.prefix = prefix
        self.features = features

    def _mount_repo(self, client: dagger.Client, ctr: dagger.Container, repo_root: pathlib.Path) -> dagger.Container:
        host_repo_root = pathlib.Path(__file__).parent.parent.parent.as_posix()
        exclude_dirs = ["build", "fluent-bit/build"]

        # The installer builds/stores intermediate artifacts under externaldeps/
        # We add a volume to speed up rebuilds. The volume has to be unique
        # per platform/distro in order to avoid mixing unrelated artifacts
        # together.
        externaldeps = self.distro._cache_volume(client, self.platform, "externaldeps")

        ctr = (
            ctr.with_directory(self.repo_root.as_posix(), client.host().directory(host_repo_root))
               .with_workdir(self.repo_root.as_posix())
               .with_mounted_cache(os.path.join(self.repo_root, "externaldeps"), externaldeps)
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

        if FeatureFlags.LogsManagement not in self.features:
            args.append("--disable-logsmanagement")

        if FeatureFlags.MachineLearning not in self.features:
            args.append("--disable-ml")

        if FeatureFlags.BundledProtobuf not in self.features:
            args.append("--use-system-protobuf")

        args.extend(["--install-prefix", self.prefix.as_posix()])


        ctr = self._mount_repo(client, ctr, self.repo_root.as_posix())

        ctr = (
            ctr.with_env_variable('NETDATA_CMAKE_OPTIONS', '-DCMAKE_BUILD_TYPE=Debug')
               .with_exec(["./netdata-installer.sh"] + args)
        )

        # The installer will place everything under "<install-prefix>/netdata"
        if self.prefix != "/":
            self.prefix = self.prefix / "netdata"

        return ctr


class ChildStreamConf:
    def __init__(self, installer: NetdataInstaller, destination: str, api_key: uuid.UUID):
        self.installer = installer
        self.substitutions = {
            "enabled": "yes",
            "destination": destination,
            "api_key": api_key,
            "timeout_seconds": 60,
            "default_port": 19999,
            "send_charts_matching": "*",
            "buffer_size_bytes": 1024 * 1024,
            "reconnect_delay_seconds": 5,
            "initial_clock_resync_iterations": 60,
        }

    def render(self) -> str:
        tmpl_path = pathlib.Path(__file__).parent / "child_stream.conf"
        with open(tmpl_path) as fp:
            tmpl = jinja2.Template(fp.read())

        return tmpl.render(**self.substitutions)


class ParentStreamConf:
    def __init__(self, installer: NetdataInstaller, api_key: str):
        self.installer = installer
        self.substitutions = {
            "api_key": api_key,
            "enabled": "yes",
            "allow_from": "*",
            "default_history": 3600,
            "health_enabled_by_default": "auto",
            "default_postpone_alarms_on_connect_seconds": 60,
            "multiple_connections": "allow",
        }

    def render(self) -> str:
        tmpl_path = pathlib.Path(__file__).parent / "parent_stream.conf"
        with open(tmpl_path) as fp:
            tmpl = jinja2.Template(fp.read())

        return tmpl.render(**self.substitutions)


class StreamConf:
    def __init__(self, child_conf: ChildStreamConf, parent_conf: ParentStreamConf):
        self.child_conf = child_conf
        self.parent_conf = parent_conf

    def render(self) -> str:
        child_section = self.child_conf.render() if self.child_conf else ''
        parent_section = self.parent_conf.render() if self.parent_conf else ''
        return '\n'.join([child_section, parent_section])


class Agent:
    def __init__(self, installer: NetdataInstaller):
        self.identifier = uuid.uuid4()
        self.installer = installer

    def _binary(self) -> pathlib.Path:
        return os.path.join(self.installer.prefix, "usr/sbin/netdata")
        
    def buildinfo(self, ctr: dagger.Container, installer: NetdataInstaller, output: pathlib.Path) -> dagger.Container:
        ctr = (
            ctr.with_exec([self._binary(), "-W", "buildinfo"], redirect_stdout=output)
        )

        return ctr

    def unittest(self, ctr: dagger.Container) -> dagger.Container:
        ctr = (
            ctr.with_exec([self._binary(), "-W", "unittest"])
        )

        return ctr

    def run(self, client: dagger.Client, ctr: dagger.Container, stream_conf: StreamConf, port, parent) -> dagger.Container:
        # Write stream.conf
        if stream_conf:
            host_stream_conf_path = str(self.identifier) + ".stream.conf"

            with open(host_stream_conf_path, 'w') as fp:
                fp.write(stream_conf.render())

            dest = self.installer.prefix / "etc/netdata/stream.conf"
            ctr = (
                ctr.with_file(dest.as_posix(), client.host().file(host_stream_conf_path))
            )

        if parent:
            ctr = ctr.with_service_binding("tilestora", parent)

        # Exec the binary
        ctr = (
            ctr.with_exposed_port(port)
               .with_exec([self._binary(), "-D", "-i", "0.0.0.0", "-p", str(port)])
        )

        return ctr


class Digraph:
    def __init__(self):
        self.nodes  = {}  # Stores Agent instances
        self.children_of = {}  # Stores children: {parent_id: [child_ids]}
        self.parents_of = {}   # Stores parents: {child_id: [parent_ids]}

    def add_node(self, node):
        self.nodes[node.identifier] = node
        if node.identifier not in self.children_of:
            self.children_of[node.identifier] = []
        if node.identifier not in self.parents_of:
            self.parents_of[node.identifier] = []

    def add_children(self, node, children):
        if node.identifier not in self.nodes :
            raise ValueError("Node not found")

        for child in children:
            if child.identifier not in self.nodes :
                raise ValueError("Child node not found")
            if node.identifier not in self.children_of[child.identifier]:
                self.children_of[node.identifier].append(child.identifier)
            if child.identifier not in self.parents_of[node.identifier]:
                self.parents_of[child.identifier].append(node.identifier)

    def get_children(self, node):
        return [self.nodes [child_id] for child_id in self.children_of.get(node.identifier, [])]

    def get_parents(self, node):
        return [self.nodes [parent_id] for parent_id in self.parents_of.get(node.identifier, [])]

    def get_siblings(self, node):
        siblings = set()
        for parent_id in self.parents_of.get(node.identifier, []):
            siblings.update(self.children_of.get(parent_id, []))
        siblings.discard(node.identifier)
        return [self.nodes [sibling_id] for sibling_id in siblings]

    def render(self, filename="digraph"):
        import graphviz
        dot = graphviz.Digraph(comment='Agent Topology')
        for identifier, node in self.nodes.items():
            dot.node(str(identifier), label=str(identifier))

        for parent_id, children_ids in self.children_of.items():
            for child_id in children_ids:
                dot.edge(str(parent_id), str(child_id))

        dot.render(filename, format='svg', cleanup=True)


class Context:
    def __init__(self,
                 client: dagger.Client,
                 platform: dagger.Platform,
                 distro: Distribution,
                 installer: NetdataInstaller,
                 agent: Agent):
        self.client = client
        self.platform = platform
        self.distro = distro
        self.installer = installer
        self.agent = agent

        self.built_distro = False
        self.built_agent = False

    def build_distro(self) -> dagger.Container:
        ctr = self.distro.build(self.client, self.platform)
        self.built_distro = True
        return ctr

    def build_agent(self, ctr: dagger.Container) -> dagger.Container:
        if not self.built_distro:
            ctr = self.build_distro()

        ctr = self.installer.install(self.client, ctr)
        self.built_agent = True
        return ctr

    def buildinfo(self, ctr: dagger.Container, output: pathlib.Path) -> dagger.Container:
        if self.built_agent == False:
            self.build_agent(ctr)

        ctr = self.agent.buildinfo(ctr, self.installer, output)
        return ctr

    def exec(self, ctr: dagger.Container) -> dagger.Container:
        if self.built_agent == False:
            self.build_agent(ctr)

        ctr = self.agent.run(ctr)
        return ctr


def run_async(func):
    def wrapper(*args, **kwargs):
        return asyncio.run(func(*args, **kwargs))
    return wrapper


@run_async
async def main():
    config = dagger.Config(log_output=sys.stdout)

    async with dagger.Connection(config) as client:
        platform = dagger.Platform("linux/x86_64")
        distro = Distribution("debian10")

        repo_root = pathlib.Path("/netdata")
        prefix_path = pathlib.Path("/opt")
        installer = NetdataInstaller(platform, distro, repo_root, prefix_path, FeatureFlags.DBEngine)
        parent_agent = Agent(installer)
        child_agent = Agent(installer)

        ctx = Context(client, platform, distro, installer, parent_agent)

        # build base image with packages we need
        ctr = ctx.build_distro()

        # build agent from source
        ctr = ctx.build_agent(ctr)

        # get the buildinfo
        # output = os.path.join(installer.prefix, "buildinfo.log")
        # ctr = ctx.buildinfo(ctr, output)

        api_key = uuid.uuid4()

        def setup_parent():
            child_stream_conf = None
            parent_stream_conf = ParentStreamConf(installer, api_key)
            stream_conf = StreamConf(child_stream_conf, parent_stream_conf)
            return stream_conf
            
        parent_stream_conf = setup_parent()
        parent = parent_agent.run(client, ctr, parent_stream_conf, 19999, None)
        parent_service = parent.as_service()

        def setup_child():
            child_stream_conf = ChildStreamConf(installer, "tilestora:19999", api_key)
            parent_stream_conf = None
            stream_conf = StreamConf(child_stream_conf, parent_stream_conf)
            return stream_conf

        child_stream_conf = setup_child()
        child = child_agent.run(client, ctr, child_stream_conf, 20000, parent_service)
        
        tunnel = await client.host().tunnel(parent_service, native=True).start()
        endpoint = await tunnel.endpoint()

        await child

        # await child.with_service_binding("tilestora", parent_service)
        # await child.with_service_binding("tilestora", parent_service)

        

        # tunnel = await client.host().tunnel(parent_service, native=True).start()
        # endpoint = await tunnel.endpoint()

        # tunnel = await client.host().tunnel(child_service, native=True).start()
        # endpoint = await tunnel.endpoint()

        time.sleep(600)

        # run unittests
        # ctr = agent.unittest(ctr)
        # await ctr


if __name__ == '__main__':
    # agent1 = Agent("Data1")
    # agent2 = Agent("Data2")
    # agent3 = Agent("Data3")
    # agent4 = Agent("Data4")

    # dg = Digraph()
    # dg.add_node(agent1)
    # dg.add_node(agent2)
    # dg.add_node(agent3)
    # dg.add_node(agent4)

    # dg.add_children(agent1, [agent2, agent3])
    # dg.add_children(agent4, [agent2, agent3])

    # dg.render()

    main()
