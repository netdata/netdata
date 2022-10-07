#!/usr/bin/env python3

import os
import sys
import json
import signal
import shutil
import subprocess
import time
import uuid

from string import Template

import click

memory_modes = [
    "dbengine", "ram", "save", "map", "alloc", "none",
]

def expand_template(src, dst, subs):
    with open(src) as fp:
        text = fp.read()
    t = Template(text).substitute(subs)

    with open(dst, "w") as fp:
        fp.write(t)

class Config:
    def __init__(self, executable, uid, web_port, stream_ports,
                 mode="dbengine", replication="no", rm="yes"):
        self.options = {
            "executable": executable,
            "uid": uid,
            "web-port": web_port,
            "stream-ports": stream_ports,
            "mode": mode,
            "replication": replication,
            "rm": rm,
        }

        self.hostname = f"nd{uid}"

        self.rfs = os.path.join(os.getcwd(), self.hostname)
        self.config_directory = os.path.join(self.rfs, "etc/netdata")
        self.log_directory = os.path.join(self.rfs, "var/log/netdata")
        self.cache_directory = os.path.join(self.rfs, "var/cache/netdata")
        self.lib_directory = os.path.join(self.rfs, "var/lib/netdata")

        self.netdata_config_file = os.path.realpath(os.path.join(self.rfs, os.pardir, f"nd{uid}.conf"))
        self.stream_config_file = os.path.join(self.config_directory, "stream.conf")
        self.registry_id_file = os.path.join(self.lib_directory, 'registry', 'netdata.public.unique.id')

        self.netdata_opts = {
            "hostname": self.hostname,
            "config_directory": self.config_directory,
            "log_directory": self.log_directory,
            "cache_directory": self.cache_directory,
            "lib_directory": self.lib_directory,
            "memory_mode": mode,
            "replication": replication,
            "web_port": web_port,
        }

        self.stream_opts = {
            "destinations": " ".join(["tcp:localhost:" + str(port) for port in stream_ports])
        }

    def generate(self):
        if self.options["rm"]:
            try:
                shutil.rmtree(self.rfs)
            except:
                pass

        os.makedirs(self.config_directory, exist_ok=True)
        os.makedirs(self.log_directory, exist_ok=True)
        os.makedirs(self.cache_directory, exist_ok=True)
        os.makedirs(self.lib_directory, exist_ok=True)
        os.makedirs(os.path.dirname(self.registry_id_file), exist_ok=True)

        expand_template("netdata.conf.tmpl", self.netdata_config_file, self.netdata_opts)
        expand_template("stream.conf.tmpl", self.stream_config_file, self.stream_opts)

        with open(self.registry_id_file, "w") as fp:
            fp.write(str(uuid.uuid3(uuid.NAMESPACE_DNS, self.hostname)))


    def __str__(self):
        d = {
            "opts": self.options,
            "netdata_opts": self.netdata_opts,
            "stream_opts": self.stream_opts,
        }
        return json.dumps(d, indent=4)

@click.command()
@click.option('--executable', required=True, type=str)
@click.option('--uid', required=True, type=int)
@click.option('--web-port', required=True, type=int)
@click.option('--stream-ports', required=True, type=int, multiple=True)
@click.option('--mode', required=False, type=click.Choice(memory_modes), default=memory_modes[0])
@click.option('--replication', required=False, type=bool, default=False)
@click.option('--rm', required=False, type=bool, default=False)
def main(executable, uid, web_port, stream_ports,
         mode, replication, rm):

    if replication:
        replication = "yes"
    else:
        replication = "no"

    cfg = Config(
        executable, uid, web_port, stream_ports,
        mode, str(replication).lower(), rm
    )
    print(cfg)

    cfg.generate()

if __name__ == '__main__':
    main()
