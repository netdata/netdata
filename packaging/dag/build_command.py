import click
import asyncio
import sys
import dagger
import pathlib
import uuid

from nd import (
    Distribution,
    NetdataInstaller,
    FeatureFlags,
    Endpoint,
    AgentContext,
    SUPPORTED_PLATFORMS,
    SUPPORTED_DISTRIBUTIONS,
)


def run_async(func):
    def wrapper(*args, **kwargs):
        return asyncio.run(func(*args, **kwargs))

    return wrapper


@run_async
async def simple_build(platform, distro):
    config = dagger.Config(log_output=sys.stdout)

    async with dagger.Connection(config) as client:
        repo_root = pathlib.Path("/netdata")
        prefix_path = pathlib.Path("/opt/netdata")

        installer = NetdataInstaller(
            platform, distro, repo_root, prefix_path, FeatureFlags.DBEngine
        )

        endpoint = Endpoint("node", 19999)
        api_key = uuid.uuid4()
        allow_children = False

        agent_ctx = AgentContext(
            client, platform, distro, installer, endpoint, api_key, allow_children
        )

        await agent_ctx.build_container()


@click.command()
@click.option(
    "--platform",
    "-p",
    type=click.Choice(sorted([str(p) for p in SUPPORTED_PLATFORMS])),
    help="Specify the platform.",
)
@click.option(
    "--distribution",
    "-d",
    type=click.Choice(sorted([str(p) for p in SUPPORTED_DISTRIBUTIONS])),
    help="Specify the distribution.",
)
def build(platform, distribution):
    platform = dagger.Platform(platform)
    distro = Distribution(distribution)
    simple_build(platform, distro)
