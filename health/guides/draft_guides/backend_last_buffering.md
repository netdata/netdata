# backend_last_buffering

**Netdata | Exporting engine**

The Netdata Agent monitors the number of seconds since the last successful buffering of backend data. This alert
indicates that the backend destination is down or unreachable. Receiving this alert means that the backend subsystem
failed to buffer metrics for a while, and some metrics are lost while exporting. Short-term network availability issues
might be fixed by increasing `buffer on failures` value in netdata.conf.

![](https://drive.google.com/uc?export=view&id=1elXR92OQn3sWVGXUCjpGi-NwcLNYE24g)
