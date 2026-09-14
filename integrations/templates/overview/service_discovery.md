[# Jinja template fragment: integrations/templates/overview/service_discovery.md #]
# [[ entry.meta.name ]] discovery

Kind: `[[ entry.meta.kind ]]`

## Overview

[[ entry.overview.description ]]

[% if entry.overview.how_it_works %]
### How it works

[[ entry.overview.how_it_works ]]

[% endif %]
[% if entry.overview.limitations %]
### Limitations

[[ entry.overview.limitations ]]
[% endif %]
