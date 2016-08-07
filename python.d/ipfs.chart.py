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
ORDER = ['bandwidth', 'peers']

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
        ]}
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

    def _get_bandwidth(self):
        """
        Format data received from http request
        :return: int, int
        """
        self.url = self.baseurl + "/api/v0/stats/bw"
        try:
            raw = self._get_raw_data()
        except AttributeError:
            return None

        try:
            parsed = json.loads(raw)
            bw_in = int(parsed['RateIn'])
            bw_out = int(parsed['RateOut'])
        except:
            return None

        return bw_in, bw_out

    def _get_peers(self):
        """
        Format data received from http request
        :return: int
        """
        self.url = self.baseurl + "/api/v0/swarm/peers"
        try:
            raw = self._get_raw_data()
        except AttributeError:
            return None

        try:
            parsed = json.loads(raw)
            peers = len(parsed['Strings'])
        except:
            return None

        return peers

    def _get_data(self):
        """
        Get data from API
        :return: dict
        """
        try:
            peers = self._get_peers()
            bandwidth_in, bandwidth_out = self._get_bandwidth()
        except:
            return None
        data = {}
        if peers is not None:
            data['peers'] = peers
        if bandwidth_in is not None and bandwidth_out is not None:
            data['in'] = bandwidth_in
            data['out'] = bandwidth_out

        if len(data) == 0:
            return None
        return data
