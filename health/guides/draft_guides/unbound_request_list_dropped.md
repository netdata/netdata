# unbound_request_list_dropped

**DNS | Unbound**

The Netdata Agent monitors the number of dropped queries. This alert indicates that Unbound is dropping new incoming
requests because the request queue is full. It can indicate a Denial of Service attack. You can try to increase the 
queue length, adjust the `num-queries-per-thread` value.

![](https://drive.google.com/uc?export=view&id=1elXR92OQn3sWVGXUCjpGi-NwcLNYE24g)
