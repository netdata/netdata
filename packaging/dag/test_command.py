import click
import asyncio
import sys
import pathlib
import dagger
import uuid
import httpx

from nd import Distribution, NetdataInstaller, FeatureFlags, Endpoint, AgentContext


def run_async(func):
    def wrapper(*args, **kwargs):
        return asyncio.run(func(*args, **kwargs))

    return wrapper


@run_async
async def simple_test():
    config = dagger.Config(log_output=sys.stdout)

    async with dagger.Connection(config) as client:
        platform = dagger.Platform("linux/x86_64")
        distro = Distribution("debian10")

        repo_root = pathlib.Path("/netdata")
        prefix_path = pathlib.Path("/opt/netdata")
        installer = NetdataInstaller(
            platform, distro, repo_root, prefix_path, FeatureFlags.DBEngine
        )

        api_key = uuid.uuid4()

        #
        # parent
        #
        parent_endpoint = Endpoint("parent1", 22000)
        parent_ctx = AgentContext(
            client, platform, distro, installer, parent_endpoint, api_key, True
        )
        parent_cmd = installer.prefix / "usr/sbin/netdata"
        parent_args = [
            parent_cmd.as_posix(),
            "-D",
            "-i",
            "0.0.0.0",
            "-p",
            str(parent_endpoint.port),
        ]

        parent_ctr = parent_ctx.build_container()
        parent_ctr = parent_ctr.with_exec(parent_args)
        parent_svc = parent_ctr.as_service()

        #
        # child
        #
        child_endpoint = Endpoint("child1", 21000)
        child_ctx = AgentContext(
            client, platform, distro, installer, child_endpoint, api_key, False
        )
        child_ctx.add_parent(parent_ctx)
        child_cmd = installer.prefix / "usr/sbin/netdata"
        child_args = [
            child_cmd.as_posix(),
            "-D",
            "-i",
            "0.0.0.0",
            "-p",
            str(child_endpoint.port),
        ]

        child_ctr = child_ctx.build_container()
        child_ctr = child_ctr.with_service_binding(parent_endpoint.hostname, parent_svc)
        child_ctr = child_ctr.with_exec(child_args)
        child_svc = child_ctr.as_service()

        #
        # endpoints
        #
        parent_tunnel, child_tunnel = await asyncio.gather(
            client.host().tunnel(parent_svc, native=True).start(),
            client.host().tunnel(child_svc, native=True).start(),
        )

        parent_endpoint, child_endpoint = await asyncio.gather(
            parent_tunnel.endpoint(),
            child_tunnel.endpoint(),
        )

        await asyncio.sleep(10)

        #
        # run tests
        #

        async with httpx.AsyncClient() as http:
            resp = await http.get(f"http://{parent_endpoint}/api/v1/info")

        #
        # Check that the child was connected
        #
        jd = resp.json()
        assert (
            "hosts-available" in jd
        ), "Could not find 'host-available' key in api/v1/info"
        assert jd["hosts-available"] == 2, "Child did not connect to parent"

        #
        # Check bearer protection
        #
        forbidden_urls = [
            f"http://{parent_endpoint}/api/v2/bearer_protection",
            f"http://{parent_endpoint}/api/v2/bearer_get_token",
        ]

        for url in forbidden_urls:
            async with httpx.AsyncClient() as http:
                resp = await http.get(url)
            assert (
                resp.status_code == httpx.codes.UNAVAILABLE_FOR_LEGAL_REASONS
            ), "Bearer protection is broken"


@click.command(help="Run a simple parent/child test")
def test():
    simple_test()
