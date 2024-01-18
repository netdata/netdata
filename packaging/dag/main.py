#!/usr/bin/env python3

import asyncio
import enum
import click
import os
import sys
import time

import anyio

import dagger
from typing import Callable, List, Tuple

import images as oci_images

import pathlib


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
            ctr.with_directory(self.repo_root, client.host().directory(host_repo_root))
               .with_workdir(self.repo_root)
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

        args.extend(["--install-prefix", self.prefix])


        ctr = self._mount_repo(client, ctr, self.repo_root)

        ctr = (
            ctr.with_env_variable('NETDATA_CMAKE_OPTIONS', '-DCMAKE_BUILD_TYPE=Debug')
               .with_exec(["./netdata-installer.sh"] + args)
        )

        # The installer will place everything under "<install-prefix>/netdata"
        if self.prefix != "/":
            self.prefix = os.path.join(self.prefix, "netdata")

        return ctr


class Agent:
    def __init__(self, installer: NetdataInstaller):
        self.installer = installer

    def buildinfo(self, ctr: dagger.Container, installer: NetdataInstaller, output: pathlib.Path) -> dagger.Container:
        binary = os.path.join(installer.prefix, "usr/sbin/netdata")

        ctr = (
            ctr.with_exec([binary, "-W", "buildinfo"], redirect_stdout=output)
        )

        return ctr

    def unittest(self, ctr: dagger.Container) -> dagger.Container:
        binary = os.path.join(self.installer.prefix, "usr/sbin/netdata")

        ctr = (
            ctr.with_exec([binary, "-W", "unittest"])
        )

        return ctr


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
        installer = NetdataInstaller(platform, distro, "/netdata", "/opt", FeatureFlags.DBEngine)
        agent = Agent(installer)

        ctx = Context(client, platform, distro, installer, agent)

        # build base image with packages we need
        ctr = ctx.build_distro()

        # build agent from source
        ctr = ctx.build_agent(ctr)

        # get the buildinfo
        output = os.path.join(installer.prefix, "buildinfo.log")
        ctr = ctx.buildinfo(ctr, output)

        # run unittests
        ctr = agent.unittest(ctr)

        await ctr


if __name__ == '__main__':
    main()
