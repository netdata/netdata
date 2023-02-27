#!/usr/bin/env python3
'''Check if a given distro is going to be EOL soon.

   This queries the public API of https://endoflife.date to fetch EOL dates.

   ‘soon’ is defined by LEAD_DAYS, currently 30 days.'''

import datetime
import json
import sys
import urllib.request

URL_BASE = 'https://endoflife.date/api'
NOW = datetime.date.today()
LEAD_DAYS = datetime.timedelta(days=30)

DISTRO = sys.argv[1]
RELEASE = sys.argv[2]


with urllib.request.urlopen(f'{ URL_BASE }/{ DISTRO }/{ RELEASE }.json') as response:
    match response.status:
        case 200:
            data = json.load(response)
        case 404:
            sys.exit(f'No data available for { DISTRO } { RELEASE }.')
        case _:
            sys.exit(
                f'Failed to retrieve data for { DISTRO } { RELEASE } ' +
                f'(status: { response.status }).'
            )

eol = datetime.date.fromisoformat(data['eol'])

offset = abs(eol - NOW)

if offset <= LEAD_DAYS:
    print(data['eol'])
    sys.exit(2)
else:
    sys.exit(0)
