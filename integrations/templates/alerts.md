## Alerts

[% if entry.alerts %]

The following alerts are available:

|  Alert name  | On metric | Description |
|:------------:|:---------:|:-----------:|
[% for alert in entry.alerts %]
| [ [[ alert.name ]] ]([[ alert.link ]]) | [[ alert.metric ]] | [[ alert.info ]] |
[% endfor %]
[% else %]
There are no alerts configured by default for this integration.
[% endif %]
