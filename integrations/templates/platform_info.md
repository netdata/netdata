[% if entries %]
The following releases of this platform are supported:

| Version | Support Tier | Native Package Architectures | Notes |
|:-------:|:------------:|:----------------------------:|:----- |
[% for e in entries %]
| [[ e.version ]] | [[ e.support ]] | [[ ', '.join(e.arches) ]] | [[ e.notes ]] |
[% endfor %]
[% endif %]
