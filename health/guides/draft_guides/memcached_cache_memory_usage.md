# memcached_cache_memory_usage

**KV Storage | Memcached**

The Netdata Agent calculates the percentage of used cached memory. This alert indicates high cache memory utilization.
If you are getting close to 100%, you will probably start experiencing evictions. Consider increasing the cache size.

This alert is triggered in warning state when the cache memory utilization is between 70-80% and in critical state when
it is between 80-90%.

![](https://drive.google.com/uc?export=view&id=1elXR92OQn3sWVGXUCjpGi-NwcLNYE24g)