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


def container_from_image(client, platform, image, use_package_builder):
    if use_package_builder:
        ctr = client.container(platform=platform).from_("netdata/package-builders:" + image)
    else:
        ctr = image.build(client, platform=platform)

    return ctr


def build_image_for_platform(client, platform, image, use_package_builder):
    repo_path = str(Path(__file__).parent.parent.parent)
    cmake_build_release_path = os.path.join(repo_path, "cmake-build-release")
    tag = platform.replace('/', '_') + '_' + image.cli_name

    externaldeps_cache = client.cache_volume(f"{tag}-externaldeps")
    fluent_bit_cache = client.cache_volume(f"{tag}-fluent_bit_build")

    ctr = container_from_image(client, dagger.Platform(platform), image, use_package_builder)

    source = (
        ctr.with_directory("/netdata", client.host().directory(repo_path), exclude=[
                    f"{cmake_build_release_path}/*",
                    "fluent-bit/build",
            ])
           .with_mounted_cache("/netdata/externaldeps", externaldeps_cache)
           .with_mounted_cache("/netdata/fluent-bit/build", fluent_bit_cache)
           .with_env_variable('CFLAGS', '-Wall -Wextra -g -O0')
    )

    return source

    enable_ml = image.cli_name != "centos7"
    build_task = source.with_workdir("/netdata").with_exec(netdata_installer(enable_ml=enable_ml))

    shell_cmd = "/opt/netdata/usr/sbin/netdata -W buildinfo | tee /opt/netdata/buildinfo.log"
    buildinfo_task = build_task.with_exec(["sh", "-c", shell_cmd])

    build_dir = buildinfo_task.directory('/opt/netdata')
    artifact_dir = os.path.join(Path.home(), f'ci/{tag}-netdata')
    output_task = build_dir.export(artifact_dir)

    return output_task


def build_images(client, platforms: List[str], images: List[str], use_package_builder):
    tasks = []

    for platform in platforms:
        for image in images:
            print(f"Building {platform=}, {image}")
            task = build_image_for_platform(client, platform, image, use_package_builder)
            tasks.append(task)

    return tasks


def validate_platforms(ctx, param, value):
    valid_platforms = set(SUPPORTED_PLATFORMS)
    input_platforms = set(value)
    if not input_platforms.issubset(valid_platforms):
        raise click.BadParameter(f"Unsupported platforms: {input_platforms - valid_platforms}")
    return value


def validate_images(ctx, param, value):
    if isinstance(value, str):
        raise click.BadParameter(f"Expected OCI list but got a single string: {value}")

    images = set()

    for v in value:
        added = False

        for img in oci_images.SUPPORTED_IMAGES:
            if v != img.cli_name:
                continue

            images.add(img)
            added = True
            break

        if not added:
            raise click.BadParameter(f"Unsupported OCI image: {v}")

    return images


def help_command():
    msg = """Build the agent with dagger.

    The script supports building the following images:

    {}

    for the following platforms:

    {}
"""
    image_list = ', '.join([str(img) for img in sorted(list(oci_images.SUPPORTED_IMAGES))])
    platform_list = ', '.join(sorted(SUPPORTED_PLATFORMS))

    return msg.format(image_list, platform_list)


def run_async(func):
    """
    Decorator to create an asynchronous runner for the main function.
    """
    def wrapper(*args, **kwargs):
        return asyncio.run(func(*args, **kwargs))
    return wrapper


@click.command(help=help_command())
@click.option(
    "--platforms",
    "-p",
    multiple=True,
    default=["linux/x86_64"],
    show_default=True,
    callback=validate_platforms,
    type=str,
    help='Space separated list of platforms to build for.',
)
@click.option(
    "--images",
    "-i",
    multiple=True,
    default=["debian12"],
    show_default=True,
    callback=validate_images,
    type=str,
    help="Space separated list of images to build.",
)
@click.option(
    "--concurrent",
    "-c",
    is_flag=True,
    default=False,
    show_default=True,
    help="Build the specified images concurrently."
)
@click.option(
    "--use-package-builders",
    is_flag=True,
    default=False,
    show_default=True,
    help="Use package builder from helper images."
)
@click.option(
    "--all-images",
    is_flag=True,
    default=False,
    show_default=True,
    help="Build all images."
)
@run_async
async def main(platforms, images, concurrent, use_package_builders, all_images):
    repo_path = str(Path(__file__).parent.parent.parent)
    cmake_build_release_path = os.path.join(repo_path, "cmake-build-release")

    config = dagger.Config(log_output=sys.stdout)
    async with dagger.Connection(config) as client:
        await oci_images.static_build(client, repo_path)
    
    # platforms = list(platforms) if platforms else SUPPORTED_PLATFORMS
    # images = list(images) if images else oci_images.SUPPORTED_IMAGES
    # use_package_builder = True

    # if all_images:
    #     images = oci_images.SUPPORTED_IMAGES

    # config = dagger.Config(log_output=sys.stdout)
    # async with dagger.Connection(config) as client:
    #     tasks = build_images(client, platforms, images, use_package_builders)

    #     if concurrent:
    #         await asyncio.gather(*tasks)
    #     else:
    #         for task in tasks:
    #             await task


if __name__ == '__main__':
    main()
