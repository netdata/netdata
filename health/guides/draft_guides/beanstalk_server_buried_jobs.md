# beanstalk_server_buried_jobs

**Messaging | Beanstalk**

The Netdata Agent monitors the number of buried jobs across all tubes. This alert means that there are buried jobs. It
usually happens if something goes wrong while the consumer processes it. The presence of buried jobs in a tube does not
affect new jobs. You need to manually kick the jobs so, they can be processed.

This alert is triggered in warning state if the number of buried jobs is more than 0 and in critical state when it is
more than 10.

![](https://drive.google.com/uc?export=view&id=1elXR92OQn3sWVGXUCjpGi-NwcLNYE24g)