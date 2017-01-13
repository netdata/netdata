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
        'options': [None, 'IPFS Repo Size', 'GB', 'Size', 'ipfs.repo_size', 'area'],
        'lines': [
            ["avail", None, "absolute", 1, 1e9],
            ["size", None, "absolute", 1, 1e9],
        ]},
    'repo_objects': {
        'options': [None, 'IPFS Repo Objects', 'objects', 'Objects', 'ipfs.repo_objects', 'line'],
        'lines': [
            ["objects", None, "absolute", 1, 1],
            ["pinned", None, "absolute", 1, 1],
            ["recursive_pins", None, "absolute", 1, 1]
        ]},
}

SI_zeroes = {'k': 3, 'm': 6, 'g': 9, 't': 12,
             'p': 15, 'e': 18, 'z': 21, 'y': 24 }


class Service(UrlService):
    def __init__(self, configuration=None, name=None):
        UrlService.__init__(self, configuration=configuration, name=name)
        try:
            self.baseurl = str(self.configuration['url'])
        except (KeyError, TypeError):
            self.baseurl = "http://localhost:5001"
        self.order = ORDER
        self.definitions = CHARTS
        self.__storagemax = None

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

    def _dehumanize(self, storemax):
        # convert from '10Gb' to 10000000000
        if type(storemax) != int:
            storemax = storemax.lower()
            if storemax.endswith('b'):
                val, units = storemax[:-2], storemax[-2]
                if units in SI_zeroes:
                    val += '0'*SI_zeroes[units]
                storemax = val
            try:
                storemax = int(storemax)
            except:
                storemax = None
        return storemax

    def _storagemax(self, storecfg):
        if self.__storagemax is None:
            self.__storagemax = self._dehumanize(storecfg['StorageMax'])
        return self.__storagemax

    def _get_data(self):
        """
        Get data from API
        :return: dict
        """
        cfg = { # suburl : List of (result-key, original-key, transform-func)
               '/api/v0/stats/bw'   :[('in', 'RateIn', int ),
                                      ('out', 'RateOut', int )],
               '/api/v0/swarm/peers':[('peers', 'Strings', len )],
               '/api/v0/stats/repo' :[('size', 'RepoSize', int),
                                      ('objects', 'NumObjects', int)],
               '/api/v0/pin/ls': [('pinned', 'Keys', len),
                                  ('recursive_pins', 'Keys', self._recursive_pins)],
               '/api/v0/config/show': [('avail', 'Datastore', self._storagemax)]
        }
        r = {}
        for suburl in cfg:
            json = self._get_json(suburl)
            for newkey, origkey, xmute in cfg[suburl]:
                try:
                    r[newkey] = xmute(json[origkey])
                except: pass
        return r or None


