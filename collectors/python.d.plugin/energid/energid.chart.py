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
    'timeoffset',
    'utxo',
    'xfers',
]

CHARTS = {
    'blockindex': {
        'options': [None, 'Blockchain Index', 'count', 'blockchain', 'energi.blockindex', 'area'],
        'lines': [
            ['blockchain_blocks', 'blocks', 'absolute'],
            ['blockchain_headers', 'headers', 'absolute'],
        ]
    },
    'difficulty': {
        'options': [None, 'Blockchain Difficulty', 'difficulty', 'blockchain', 'energi.difficulty', 'line'],
        'lines': [
            ['blockchain_difficulty', 'Diff', 'absolute'],
        ],
    },
    'mempool': {
        'options': [None, 'MemPool', 'MiB', 'memory', 'energid.mempool', 'area'],
        'lines': [
            ['mempool_max', 'Max', 'absolute', None, 1024*1024],
            ['mempool_current', 'Usage', 'absolute', None, 1024*1024],
            ['mempool_txsize', 'TX Size', 'absolute', None, 1024*1024],
        ],
    },
    'secmem': {
        'options': [None, 'Secure Memory', 'KiB', 'memory', 'energid.secmem', 'area'],
        'lines': [
            ['secmem_total', 'Total', 'absolute', None, 1024],
            ['secmem_locked', 'Locked', 'absolute', None, 1024],
            ['secmem_used', 'Used', 'absolute', None, 1024],
        ],
    },
    'network': {
        'options': [None, 'Network', 'count', 'network', 'energid.network', 'line'],
        'lines': [
            ['network_connections', 'Connections', 'absolute'],
        ],
    },
    'timeoffset': {
        'options': [None, 'Network', 'seconds', 'network', 'energid.timeoffset', 'line'],
        'lines': [
            ['network_timeoffset', 'offseet', 'absolute'],
        ],
    },
    'utxo': {
        'options': [None, 'UTXO', 'count', 'UTXO', 'energid.utxo', 'line'],
        'lines': [
            ['utxo_count', 'UTXO', 'absolute'],
        ],
    },
    'xfers': {
        'options': [None, 'UTXO', 'count', 'UTXO', 'energid.xfers', 'line'],
        'lines': [
            ['utxo_xfers', 'Xfers', 'absolute'],
        ],
    },
}

METHODS = {
    'getblockchaininfo': lambda r: {
        'blockchain_blocks': r['blocks'],
        'blockchain_headers': r['headers'],
        'blockchain_difficulty': r['difficulty'],
    },
    'getmempoolinfo': lambda r: {
        'mempool_txcount': r['size'],
        'mempool_txsize': r['bytes'],
        'mempool_current': r['usage'],
        'mempool_max': r['maxmempool'],
    },
    'getmemoryinfo': lambda r: dict([
        ('secmem_' + k, v) for (k,v) in r['locked'].items()
    ]),
    'getnetworkinfo': lambda r: {
        'network_timeoffset' : r['timeoffset'],
        'network_connections': r['connections'],
    },
    'gettxoutsetinfo': lambda r: {
        'utxo_count' : r['txouts'],
        'utxo_xfers' : r['transactions'],
        'utxo_size' : r['disk_size'],
        'utxo_amount' : r['total_amount'],
    },
}

JSON_RPC_VERSION = '1.1'

class Service(UrlService):
    def __init__(self, configuration=None, name=None):
        UrlService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = CHARTS
        self.host = self.configuration.get('host', '127.0.0.1')
        self.port = self.configuration.get('port', 9796)
        self.url = '{scheme}://{host}:{port}'.format(
            scheme=self.configuration.get('scheme', 'http'),
            host=self.host,
            port=self.port,
        )
        self.method = 'POST'
        self.header = {
            'Content-Type': 'application/json',
        }

    def _get_data(self):
        #
        # Bitcoin family speak JSON-RPC version 1.0 for maximum compatibility,
        # but uses JSON-RPC 1.1/2.0 standards for parts of the 1.0 standard that were
        # unspecified (HTTP errors and contents of 'error').
        #
        # 1.0 spec: https://www.jsonrpc.org/specification_v1
        # 2.0 spec: https://www.jsonrpc.org/specification
        #
        # The call documentation: https://github.com/energicryptocurrency/core-api-documentation
        #
        batch = []

        for i, method in enumerate(METHODS):
            batch.append({
                'version': JSON_RPC_VERSION,
                'id': i,
                'method': method,
                'params': [],
            })

        result = self._get_raw_data(body=json.dumps(batch))

        if not result:
            return None

        result = json.loads(result.decode('utf-8'))
        data = dict()

        for i, (_, handler) in enumerate(METHODS.items()):
            r = result[i]
            data.update(handler(r['result']))

        return data
