# riakkv_list_keys_active

**Database | Riak KV**

The Netdata Agent monitors the number of currently running list keys for Finite State Machines (FSM). This alert
indicates that there are active list keys FSMs. A key listing in Riak is a very expensive operation, and should not be
used in production as it will affect the performance of the cluster and not scale well.

![](https://drive.google.com/uc?export=view&id=1elXR92OQn3sWVGXUCjpGi-NwcLNYE24g)

