#!/usr/bin/env python3

import click

from test_command import test
from build_command import build


@click.group()
def cli():
    pass


cli.add_command(test)
cli.add_command(build)

if __name__ == "__main__":
    cli()
