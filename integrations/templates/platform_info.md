[% if entries %]
We build native packages for the following releases:

| Version | Support Tier | Native Package Architectures | Notes |
|:-------:|:------------:|:----------------------------:|:----- |
[% for e in entries %]
| [[ e.version ]] | [[ e.support ]] | [[ ', '.join(e.arches) ]] | [[ e.notes ]] |
[% endfor %]

On other releases of this distribution, a static binary will be installed in `/opt/netdata`.
[% endif %]
