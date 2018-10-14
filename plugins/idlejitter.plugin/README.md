## idlejitter.plugin

It works like this:

A thread is spawn that requests to sleep for a few microseconds.
When the system wakes it up, it measures how many microseconds have passed.
The difference between the requested and the actual duration of the sleep, is the idle jitter.
This is done hundreds of times per second, to ensure we have a good average. 

This number is useful in real-time environments, when the CPU jitter can affect the quality of the service
(like VoIP media gateways).
