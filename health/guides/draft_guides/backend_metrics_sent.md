# backend_metrics_sent

**Netdata | Exporting engine**

The Netdata Agent calculates the percentage of metrics sent to the backend server. The backend subsystem failed to send
all metrics and, some metrics are lost while exporting. It indicates that the backend destination is down or
unreachable. Short-term network availability issues might be fixed by increasing `buffer on failures` value
in `netdata. conf`.

![](https://drive.google.com/uc?export=view&id=1elXR92OQn3sWVGXUCjpGi-NwcLNYE24g)

warn: $this != 100

crit: -

