# -*- coding: utf-8 -*-
# Description: IPFS netdata python.d module
# Authors: Pawel Krupa (paulfantom), davidak

from base import UrlService
import json

# default module values (can be overridden per job in `config`)
# update_every = 2
priority = 60000
retries = 60

# default job configuration (overridden by python.d.plugin)
# config = {'local': {
#     'update_every': update_every,
#     'retries': retries,
#     'priority': priority,
#     'url': 'http://localhost:5001'
# }}

# charts order (can be overridden if you want less charts, or different order)
ORDER = ['bandwidth', 'peers', 'repo_size', 'repo_objects']

CHARTS = {
    'bandwidth': {
        'options': [None, 'IPFS Bandwidth', 'kbits/s', 'Bandwidth', 'ipfs.bandwidth', 'line'],
        'lines': [
            ["in", None, "absolute", 8, 1000],
            ["out", None, "absolute", -8, 1000]
        ]},
    'peers': {
        'options': [None, 'IPFS Peers', 'peers', 'Peers', 'ipfs.peers', 'line'],
        'lines': [
            ["peers", None, 'absolute']
        ]},
    'repo_size': {
        'options': [None, 'IPFS Repo Size', 'MB', 'Size', 'ipfs.repo_size', 'line'],
        'lines': [
            ["size", None, "absolute", 1, 1000000]
        ]},
    'repo_objects': {
        'options': [None, 'IPFS Repo Objects', 'objects', 'Objects', 'ipfs.repo_objects', 'line'],
        'lines': [
            ["objects", None, "absolute", 1, 1],
            ["pinned", None, "absolute", 1, 1],
            ["recursive_pins", None, "absolute", 1, 1]
        ]},
}

class Service(UrlService):
    def __init__(self, configuration=None, name=None):
        UrlService.__init__(self, configuration=configuration, name=name)
        try:
            self.baseurl = str(self.configuration['url'])
        except (KeyError, TypeError):
            self.baseurl = "http://localhost:5001"
        self.order = ORDER
        self.definitions = CHARTS

    def _get_json(self, suburl):
        """
        :return: json decoding of the specified url
        """ 
        self.url = self.baseurl + suburl
        try:
            return json.loads(self._get_raw_data()) 
        except:
            return {}

    def _recursive_pins(self, keys):
        return len([k for k in keys if keys[k]["Type"] == b"recursive"])


    def _get_data(self):
        """
        Get data from API
        :return: dict
        """
        cfg = {# suburl : List of (result-key, original-key, transform-func)
               '/api/v0/stats/bw'   :[('in', 'RateIn', int ),
                                      ('out', 'RateOut', int )],
               '/api/v0/swarm/peers':[('peers', 'Strings', len )],
               '/api/v0/stats/repo' :[('size', 'RepoSize', int),
                                      ('objects', 'NumObjects', int)],
               '/api/v0/pin/ls': [('pinned', 'Keys', len),
                                  ('recursive_pins', 'Keys', self._recursive_pins)
                                 ]
        }
        r = {}
        for suburl in cfg:
            json = self._get_json(suburl)
            for newkey, origkey, xmute in cfg[suburl]:
                try:
                    r[newkey] = xmute(json[origkey])
                except: pass
        return r or None


 

