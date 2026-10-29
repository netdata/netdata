#!/usr/bin/env python3

'''Determine the status of a local copy of a Docker image.'''

import json
import subprocess
import sys

from enum import StrEnum, unique


@unique
class Status(StrEnum):
    UNKNOWN = 'UNKNOWN'
    OK = 'OK'
    MISSING = 'MISSING'
    MISMATCH = 'MISMATCH'
    UPDATE = 'UPDATE'


def result(status: Status) -> None:
    print(str(status))

    if status is Status.UNKNOWN:
        sys.exit(1)

    sys.exit(0)


def main() -> None:
    if len(sys.argv) != 4:
        raise RuntimeError('Insufficient arguments.')

    docker_cmd = sys.argv[1]
    image = sys.argv[2]
    platform = sys.argv[3].split('/')

    def platform_match(d: dict) -> bool:
        if platform[0] != d.get('Os', d.get('os', '')):
            return False

        if platform[1] != d.get('Architecture', d.get('architecture', '')):
            return False

        if len(platform) == 3:
            if platform[2] != d.get('Variant', d.get('variant', '')):
                return False
        else:
            if 'Variant' in d:
                return False

        return True

    image_info = subprocess.run(
        (
            docker_cmd,
            'image',
            'inspect',
            image,
        ),
        capture_output=True,
    )

    match image_info.returncode:
        case 0:
            parsed_image_info = json.loads(image_info.stdout)[0]

            if not platform_match(parsed_image_info):
                result(Status.MISMATCH)

            if 'Digest' in parsed_image_info:
                image_digests = [parsed_image_info['Digest']]
            elif 'RepoDigests' in parsed_image_info:
                image_digests = [
                    x.rsplit('@', 1)[-1] for x in parsed_image_info['RepoDigests']
                ]
            else:
                result(Status.UNKNOWN)

            manifest_info = subprocess.run(
                (
                    docker_cmd,
                    'manifest',
                    'inspect',
                    image,
                ),
                capture_output=True,
            )

            if manifest_info.returncode != 0:
                result(Status.UNKNOWN)

            parsed_manifest_info = json.loads(manifest_info.stdout)

            target_manifest = next(filter(
                lambda x: platform_match(x['platform']),
                parsed_manifest_info['manifests']
            ))

            if not any(map(lambda x: target_manifest['digest'] == x, image_digests)):
                result(Status.UPDATE)

            result(Status.OK)
        case 125:
            result(Status.MISSING)
        case _:
            result(Status.UNKNOWN)


if __name__ == '__main__':
    main()
