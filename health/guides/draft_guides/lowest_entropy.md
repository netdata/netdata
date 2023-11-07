# lowest_entropy

## OS: Linux

Entropy is the randomness collected by an operating system or application for use in cryptography or other uses that
require random data. The Netdata Agent monitors the number of entries in the random numbers pool in the last 5 minutes.
This alert indicates that a low number of bits of entropy is available, which may have a negative impact on the security and
performance of the system. This can be fixed by installing the `haveged` or `rngd` daemon.

![](https://drive.google.com/uc?export=view&id=1elXR92OQn3sWVGXUCjpGi-NwcLNYE24g)

