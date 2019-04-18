# -*- coding: utf-8 -*-
# Description: Energi Core / Bitcoin netdata python.d module
# Author: Andrey Galkin <andrey@futoin.org> (andvgal)
# SPDX-License-Identifier: GPL-3.0-or-later
#
# This module is designed for energid, but it should work with many other Bitcoin forks
# which support more or less standard JSON-RPC.
#

import json

from bases.FrameworkServices.UrlService import UrlService

update_every = 5

ORDER = [
    'blockindex',
    'difficulty',
    'mempool',
    'secmem',
    'network',
    'utxo',
]

CHARTS = {
    'blockindex': {
        'options': [None, 'Blockchain Index', 'count', 'blockchain', 'energi.blockindex', 'area'],
        'lines': [
            ['blockchain.blocks', 'blocks', 'absolute'],
            ['blockchain.headers', 'headers', 'absolute'],
        ]
    },
    'difficulty': {
        'options': [None, 'Blockchain Difficulty', 'difficulty', 'blockchain', 'energi.difficulty', 'line'],
        'lines': [
            ['blockchain.difficulty', 'Diff', 'absolute'],
        ],
    },
    'mempool': {
        'options': [None, 'MemPool', 'MiB', 'mempool', 'energi.mempool', 'area'],
        'lines': [
            ['mempool.max', 'Max', 'absolute', None, 1024*1024],
            ['mempool.current', 'Usage', 'absolute', None, 1024*1024],
            ['mempool.txsize', 'TX Size', 'absolute', None, 1024*1024],
        ],
    },
    'secmem': {
        'options': [None, 'Secure Memory', 'KiB', 'memory', 'energi.secmem', 'area'],
        'lines': [
            ['secmem.total', 'Total', 'absolute', None, 1024],
            ['secmem.locked', 'Locked', 'absolute', None, 1024],
            ['secmem.used', 'Used', 'absolute', None, 1024],
        ],
    },
    'network': {
        'options': [None, 'Network', 'count', 'network', 'energi.network', 'line'],
        'lines': [
            ['network.connections', 'Connections', 'absolute'],
        ],
    },
    'utxo': {
        'options': [None, 'UTXO', 'count', 'UTXO', 'energi.utxo', 'line'],
        'lines': [
            ['utxo.count', 'UTXO', 'absolute'],
            ['utxo.xfers', 'Xfers', 'absolute'],
        ],
    },
}


class Service(UrlService):
    def __init__(self, configuration=None, name=None):
        UrlService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = CHARTS
        self.host = self.configuration.get('host', '127.0.0.1')
        self.port = self.configuration.get('port', 9796)
        self.url = 'http://{host}:{port}'.format(
            host=self.host,
            port=self.port,
        )
        self.method = 'POST'
        self.header = {
            'Content-Type': 'application/json',
        }
        self.methods = {
            'getblockchaininfo': lambda r: {
                'blockchain.blocks': r['blocks'],
                'blockchain.headers': r['headers'],
                'blockchain.difficulty': r['difficulty'],
            },
            'getmempoolinfo': lambda r: {
                'mempool.txcount': r['size'],
                'mempool.txsize': r['bytes'],
                'mempool.current': r['usage'],
                'mempool.max': r['maxmempool'],
            },
            'getmemoryinfo': lambda r: dict([
                ('secmem.' + k, v) for (k,v) in r['locked'].items()
            ]),
            'getnetworkinfo': lambda r: {
                'network.timeoffset' : r['timeoffset'],
                'network.connections': r['connections'],
            },
            'gettxoutsetinfo': lambda r: {
                'utxo.count' : r['txouts'],
                'utxo.xfers' : r['transactions'],
                'utxo.size' : r['disk_size'],
                'utxo.amount' : r['total_amount'],
            },
        }

    def _get_data(self):
        batch = []

        for i, method in enumerate(self.methods.keys()):
            batch.append({
                'version': '1.1',
                'id': i,
                'method': method,
                'params': [],
            })

        result = self._get_raw_data(body=json.dumps(batch))
        result = json.loads(result.decode('utf-8'))
        data = dict()

        for i, (_, handler) in enumerate(self.methods.items()):
            r = result[i]
            assert(r['id'] == i)
            data.update(handler(r['result']))

        return data
