# -*- coding: utf-8 -*-
# Description: IPFS netdata python.d module
# Author: Pawel Krupa (paulfantom)

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
#     'url': 'http://localhost:5001/api/v0/swarm/peers'
# }}

# charts order (can be overridden if you want less charts, or different order)
ORDER = ['peers']

CHARTS = {
    'peers': {
        'options': [None, 'IPFS Peers', 'peers', 'Peers', 'ipfs.peers', 'line'],
        'lines': [
            ["peers", None, 'absolute']
        ]}
}


class Service(UrlService):
    def __init__(self, configuration=None, name=None):
        UrlService.__init__(self, configuration=configuration, name=name)
        if len(self.url) == 0:
            self.url = "http://localhost:5001/api/v0/swarm/peers"
        self.order = ORDER
        self.definitions = CHARTS

    def _get_data(self):
        """
        Format data received from http request
        :return: dict
        """
        try:
            raw = self._get_raw_data()
        except AttributeError:
            return None

        try:
            parsed = json.loads(raw)
            peers = len(parsed['Strings'])
        except:
            return None

        return {'peers': peers}
