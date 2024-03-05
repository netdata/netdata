#!/usr/bin/env python3

import sys

from pathlib import Path

DIGEST_PATH = Path(sys.argv[1])
TAG_PREFIX = sys.argv[2]
TAGS = sys.argv[3]

if TAG_PREFIX:
    PUSH_TAGS = tuple([
        t for t in TAGS.split(',') if t.startswith(TAG_PREFIX)
    ])
else:
    PUSH_TAGS = tuple([
        t for t in TAGS.split(',') if t.startswith('netdata/')
    ])

IMAGE_NAME = PUSH_TAGS[0].split(':')[0]

images = []

for f in DIGEST_PATH.glob('*'):
    images.append(f'{IMAGE_NAME}@sha256:{f.name}')

print(f'-t {" -t ".join(PUSH_TAGS)} {" ".join(images)}')
