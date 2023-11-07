# unbound_request_list_overwritten

**DNS | Unbound**

The Netdata Agent monitors the number of overwritten queries. This alert indicates that Unbound is overwriting old
queued requests because request queue is full. It can indicate a Denial of Service attack. To increase the queue length,
adjust the `num-queries-per-thread` value.

![](https://drive.google.com/uc?export=view&id=1elXR92OQn3sWVGXUCjpGi-NwcLNYE24g)

