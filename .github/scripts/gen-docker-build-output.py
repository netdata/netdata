#!/usr/bin/env python3

import sys

event = sys.argv[1]

match event:
    case 'workflow_dispatch':
        print('type=image,push=true,push-by-digest=true,name-canonical=true')
    case _:
        print('type=docker')
