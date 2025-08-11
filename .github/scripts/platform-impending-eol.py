#!/usr/bin/env python3
"""Check if a given distro is going to be EOL soon.

   This queries the public API of https://endoflife.date to fetch EOL dates.

   'soon' is defined by LEAD_DAYS, currently 30 days."""

import datetime
import json
import sys
import urllib.request

URL_BASE = 'https://endoflife.date/api'
NOW = datetime.date.today()
LEAD_DAYS = datetime.timedelta(days=30)

DISTRO = sys.argv[1]
RELEASE = sys.argv[2]
LTS = sys.argv[3]

EXIT_NOT_IMPENDING = 0
EXIT_IMPENDING = 1
EXIT_NO_DATA = 2
EXIT_FAILURE = 3

try:
    with urllib.request.urlopen(f'{URL_BASE}/{DISTRO}/{RELEASE}.json') as response:
        match response.status:
            case 200:
                data = json.load(response)
            case _:
                print(
                    f'Failed to retrieve data for {DISTRO} {RELEASE} ' +
                    f'(status: {response.status}).',
                    file=sys.stderr
                )
                sys.exit(EXIT_FAILURE)
except urllib.error.HTTPError as e:
    match e.code:
        case 404:
            print(f'No data available for {DISTRO} {RELEASE}.', file=sys.stderr)
            sys.exit(EXIT_NO_DATA)
        case _:
            print(
                f'Failed to retrieve data for {DISTRO} {RELEASE} ' +
                f'(status: {e.code}).',
                file=sys.stderr
            )
            sys.exit(EXIT_FAILURE)

if LTS == '1' and 'extendedSupport' in data:
    ref = 'extendedSupport'
else:
    ref = 'eol'
    LTS = False

# Handle boolean values for eol/extendedSupport fields
eol_value = data[ref]

# Check if the value is a boolean
if isinstance(eol_value, bool):
    if ref == 'eol':
        if not eol_value:
            # False for eol means no EOL date set (still actively supported)
            sys.exit(EXIT_NOT_IMPENDING)
        else:
            # True for eol would mean EOL has occurred (unusual, but handle it)
            # Without a date, we can't determine if it's within LEAD_DAYS
            # Conservative approach: treat as impending
            print(f'{DISTRO} {RELEASE} has reached EOL (boolean true)', file=sys.stderr)
            sys.exit(EXIT_IMPENDING)
    elif ref == 'extendedSupport':
        if not eol_value:
            # False for extendedSupport when LTS was requested is unexpected
            print(
                f'Unexpected value for {DISTRO} {RELEASE} extendedSupport: false ' +
                '(LTS was requested but extended support is not available)',
                file=sys.stderr
            )
            sys.exit(EXIT_FAILURE)
        else:
            # True for extendedSupport means LTS available but date not determined
            # Can't calculate if impending without a date
            sys.exit(EXIT_NOT_IMPENDING)

# If we get here, it should be a date string
try:
    eol = datetime.date.fromisoformat(eol_value)
except (ValueError, TypeError):
    print(
        f'Invalid date format for {DISTRO} {RELEASE} {ref}: {eol_value}',
        file=sys.stderr
    )
    sys.exit(EXIT_FAILURE)

offset = abs(eol - NOW)

if offset <= LEAD_DAYS:
    if LTS:
        print(data['extendedSupport'])
    else:
        print(data['eol'])

    sys.exit(EXIT_IMPENDING)
else:
    sys.exit(EXIT_NOT_IMPENDING)
