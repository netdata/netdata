## Alerts

[% if entry.alerts %]

The following alerts are available:

| Alert name  | On metric | Description |
|:------------|:----------|:------------|
[% for alert in entry.alerts %]
| [ [[ strfy(alert.name) ]] ]([[ strfy(alert.link) ]]) | [[ strfy(alert.metric) ]] | [[ strfy(alert.info) ]] |
[% endfor %]
[% else %]
There are no alerts configured by default for this integration.
[% endif %]
