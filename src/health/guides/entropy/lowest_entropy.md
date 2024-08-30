### Understand the alert

This alert presents the minimum amount of entropy in the kernel entropy pool in the last 5 minutes. Low entropy can lead to a reduction in the quality of random numbers produced by `/dev/random` and `/dev/urandom`.

The Netdata Agent checks for the minimum entropy value in the last 5 minutes. The alert gets raised into warning if the value < 100, and cleared if the value > 200.  

For further information on how our alerts are calculated, please have a look at our [Documentation](/src/health/REFERENCE.md#expressions).

### What is entropy and why do we need it?

Entropy is similar to "randomness". A Linux system gathers "real" random numbers by keeping an eye on different events: network activity, hard drive rotation speeds, hardware random number generator (if available), key-clicks, and so on. It feeds those to the kernel entropy pool, which is used by `/dev/random`.

Encryption and cryptography applications require random numbers to operate. A function or an algorithm that produces numbers -*that seem to be random*- is very predictable, if you know what function is used.

In real life, we use our surroundings and our thoughts to produce truly random numbers. A computer can't really do this by itself, so it gathers numbers from a lot of sources. For example, it can get the CO2 levels in a Room from a sensor on the system and use that as a random number.

This way all the values are random and there is no pattern to be found among them.

### Troubleshoot the alert

The best tool to troubleshoot the lowest entropy alert is with `rng-tools`. 

If `rng-tools` are not available for your platform, or you run into trouble, you can use the tool `haveged` as an alternative.

### Useful resources

1. [Entropy](https://unixhealthcheck.com/blog?id=472)
2. [rng-tools](https://github.com/nhorman/rng-tools)
3. [How to add more entropy to improve cryptographic randomness on Linux](https://www.techrepublic.com/article/how-to-add-more-entropy-to-improve-cryptographic-randomness-on-linux/)
4. [Haveged Installation - Archlinux Wiki](https://wiki.archlinux.org/title/Haveged#Installation)
