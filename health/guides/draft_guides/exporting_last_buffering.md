# exporting_last_buffering

**Netdata | Exporting engine**

The Netdata Agent calculates the number of seconds since the last successful buffering of exporting data. This alert
indicates that the exporting engine failed to buffer metrics for a while, and some metrics were lost while exporting.
The exporting destination may be down or unreachable. Short-term network availability problems might be fixed by
increasing `buffer on failures` value in `exporting.conf`.

![](https://drive.google.com/uc?export=view&id=1elXR92OQn3sWVGXUCjpGi-NwcLNYE24g)


