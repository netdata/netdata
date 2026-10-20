#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-or-later
"""Synthetic line snapshots. Use JSON string encoding for arbitrary labels."""
import json
import sys


def snapshot():
    print('temperature:21.5|gauge|unit:Celsius')
    queue = 'batch, "night" | café 😀'
    print(f'queue.depth:17|gauge|#queue:{json.dumps(queue)}|unit:jobs')
    print('requests_total:12345|counter|#service:api|unit:requests|title:API requests')


def main():
    operation = sys.argv[-1]
    if operation == 'collect':
        snapshot()
    elif operation == 'serve':
        print(json.dumps({'version': 'v1', 'ready': True}), flush=True)
        for line in sys.stdin:
            request = json.loads(line)
            if request['method'] != 'collect':
                raise ValueError('expected a collection request')
            snapshot()
            print(f"# EOF {request['id']}", flush=True)
    else:
        raise ValueError('expected collect or serve')


if __name__ == '__main__':
    main()
