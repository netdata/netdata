## idlejitter.plugin

It works like this:

A thread is spawn that requests to sleep for 20000 microseconds (20ms).
When the system wakes it up, it measures how many microseconds have passed.
The difference between the requested and the actual duration of the sleep, is the idle jitter.
This is done at most 50 times per second, to ensure we have a good average. 

This number is useful:
 
 1. in real-time environments, when the CPU jitter can affect the quality of the service (like VoIP media gateways).
 2. in cloud infrastructure, at can pause the VM or container for a small duration to perform operations at the host. 
