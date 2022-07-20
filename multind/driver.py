#!/usr/bin/env python3

import os
import sys
import json

import yaml

from string import Template

import genconf

import click

def to_yaml(child_agents, level_agents):
    pass


class Agent:
    def __init__(self, executable, uid, web_port, rm):
        self.executable = executable
        self.uid = uid
        self.web_port = web_port
        self.stream_ports = []
        self.rm = rm

    def link(self, agent):
        self.stream_ports.append(agent.web_port)

    def genconf(self):
        self.cfg = genconf.Config(
                self.executable,
                self.uid,
                self.web_port,
                self.stream_ports,
                "dbengine",     # mode
                "no",           # replication
                self.rm,        # rm
        )
        self.cfg.generate()
        print(self)

    def window(self):
        return {
            'layout': 'even-horizontal',
            'panes': [
                f"{self.executable} -W keepopenfds -D -c {self.cfg.netdata_config_file}",
                f"sleep 5 && cd {self.cfg.hostname}/var/log/netdata/ && less -N -S +F error.log"
            ]
        }

    def __repr__(self):
        return f'Agent(uid={self.uid}, web_port={self.web_port}, stream_ports={self.stream_ports})'

    def __str__(self):
        return f'Agent(uid={self.uid}, web_port={self.web_port}, stream_ports={self.stream_ports})'

@click.command()
@click.option('--executable', required=True, type=str)
@click.option('--childs', required=True, type=int)
@click.option('--levels', required=True, type=int)
@click.option('--active-active', required=True, type=bool)
@click.option('--rm', required=True, type=bool)
def main(executable, childs, levels, active_active, rm):
    child_agents = [
        Agent(executable, uid, 20000 + uid, rm) for uid in range(childs)
    ]

    level_agents = [
        [
            Agent(executable, 100 * (level + 1), 20000 + 100 * (level + 1), rm),
            Agent(executable, 100 * (level + 1) + 1, 20000 + 100 * (level + 1) + 1, rm)
        ] for level in range(levels)
    ]

    if not active_active:
        level_agents = [[x[0]] for x in level_agents]

    for la in level_agents:
        if len(la) == 2:
            la[0].link(la[1])
            la[1].link(la[0])

    if levels != 0:
        for ca in child_agents:
            for la in level_agents[0]:
                ca.link(la)

    for (la, next_la) in zip(level_agents, level_agents[1:]):
        for lower_level_agent in la:
            for higher_level_agent in next_la:
                lower_level_agent.link(higher_level_agent)

    for ca in child_agents:
        ca.genconf()

    for la in level_agents:
        for a in la:
            a.genconf()

    d = {
        'name': 'streaming',
        'root': os.path.realpath(os.path.dirname(__file__)),
        'windows': []
    }

    for ca in child_agents:
        d['windows'].append({ f'c{ca.uid}': ca.window() })

    for li, la in enumerate(level_agents):
        for ai, a in enumerate(la):
            d['windows'].append({ f'l{li}{ai}': a.window() })

    with open('tmux-session.yml', 'w') as outfile:
        yaml.dump(d, outfile, default_flow_style=False)


if __name__ == '__main__':
    main()
