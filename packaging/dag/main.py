#!/usr/bin/env python3

import asyncio
import click
import os
from pathlib import Path
import sys

import anyio

import dagger
from typing import List

import images as oci_images


SUPPORTED_PLATFORMS = [
    "linux/x86_64",
    "linux/arm64",
    "linux/i386",
    "linux/arm/v7",
    "linux/arm/v6",
    "linux/ppc64le",
    "linux/s390x",
    "linux/riscv64",
]


def netdata_installer(enable_ml=True, enable_ebpf=False, enable_go=False):
    cmd = [
        "./netdata-installer.sh",
        "--disable-telemetry",
        "--disable-logsmanagement"
    ]

    if not enable_ebpf:
        cmd.append("--disable-ebpf")

    if not enable_ml:
        cmd.append("--disable-ml")

    if not enable_go:
        cmd.append('--disable-go')

    cmd.extend([
        "--dont-wait",
        "--dont-start-it",
        "--install-prefix",
        "/opt"
    ])

    return cmd


def build_image_for_platform(client, image_name, platform: dagger.Platform, ctr : dagger.Container):
    repo_path = str(Path(__file__).parent.parent.parent)
    exclude_dirs = exclude=["build", "fluent-bit/build"]

    tag = image_name + "_" + str(platform).replace('/', '_')

    externaldeps_cache = client.cache_volume(f"{tag}-externaldeps")

    source = (
        ctr.with_directory("/netdata", client.host().directory(repo_path), exclude=exclude_dirs)
           .with_mounted_cache("/netdata/externaldeps", externaldeps_cache)
           .with_env_variable('NETDATA_CMAKE_OPTIONS', '-DCMAKE_BUILD_TYPE=Debug')
    )

    enable_ml = "centos7" not in image_name
    build_task = source.with_workdir("/netdata").with_exec(netdata_installer(enable_ml=enable_ml))

    shell_cmd = "/opt/netdata/usr/sbin/netdata -W buildinfo | tee /opt/netdata/buildinfo.log"
    buildinfo_task = build_task.with_exec(["sh", "-c", shell_cmd])

    build_dir = buildinfo_task.directory('/opt/netdata')
    artifact_dir = os.path.join(Path.home(), f'ci/{tag}')
    output_task = build_dir.export(artifact_dir)

    return output_task

def run_async(func):
    """
    Decorator to create an asynchronous runner for the main function.
    """
    def wrapper(*args, **kwargs):
        return asyncio.run(func(*args, **kwargs))
    return wrapper


@run_async
async def main():
    repo_path = str(Path(__file__).parent.parent.parent)

    platform = dagger.Platform("linux/x86_64")

    config = dagger.Config(log_output=sys.stdout)
    async with dagger.Connection(config) as client:
        ctr = oci_images.build_debian_12(client, platform)
        ctr = build_image_for_platform(client, "debian_12", platform, ctr)
        await ctr
        # await oci_images.static_build(client, repo_path)
    
if __name__ == '__main__':
    main()
