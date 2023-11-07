# lowest_entropy

## OS: Linux

This alert presents the minimum amount of entropy in the kernel entropy pool in the last 5 minutes.

Low entropy can lead to a reduction in the quality of random numbers produced by `/dev/random`
and `/dev/urandom`.

The Netdata Agent checks for the minimum entropy value in the last 5 minutes. The alert gets raised
into warning if the value < 100, and cleared if the value > 200.  

For further information on how our alerts are calculated, please have a look at our [Documentation](
https://learn.netdata.cloud/docs/agent/health/reference#expressions).


<details>
<summary>What is entropy and why do we need it?</summary> 

Entropy is similar to "randomness". A Linux system gathers "real" random numbers by keeping an eye
on different events: network activity, hard drive rotation speeds, hardware random number
generator (if available), key-clicks, and so on. It feeds those to the kernel entropy pool, which is
used by `/dev/random`.<sup>[1](
https://unixhealthcheck.com/blog?id=472) </sup>

Encryption and cryptography applications require random numbers to operate. A function or an
algorithm that produces numbers -*that seem to be random*- is very predictable, if you know what
function is used.

In real life, we use our surroundings and our thoughts to produce truly random numbers. A computer
can't really do this by itself, so it gathers numbers from a lot of sources. For example, it can get
the CO<sub>2</sub> levels in a room from a sensor on the system and use that as a random number.

This way all the values are random and there is no pattern to be found among them.
</details>

For further information, please have a look at the _References and Sources_ section.

<details>
<summary>References and Sources</summary>

1. [Entropy](https://unixhealthcheck.com/blog?id=472)
2. [rng-tools](https://github.com/nhorman/rng-tools)
3. [How to add more entropy to improve cryptographic randomness on Linux](
   https://www.techrepublic.com/article/how-to-add-more-entropy-to-improve-cryptographic-randomness-on-linux/)
4. [Haveged Installation - Archlinux Wiki](https://wiki.archlinux.org/title/Haveged#Installation)

</details>

### Troubleshooting Section

The best tool to troubleshoot the lowest entropy alert is with `rng-tools`. If `rng-tools` are not
available for your platform, or you run into trouble, you can use the tool `haveged` as an
alternative.

<details>
<summary>Install and setup rng-tools</summary>

`rng-tools` is a random number generator daemon.  
It monitors a set of entropy sources, and supplies entropy from them to the system kernel's
/dev/random machinery.<sup>[2](https://github.com/nhorman/rng-tools) </sup>

### Installation

### Debian-based platforms

```
root@netdata~ # sudo apt-get update
root@netdata~ # sudo apt-get install rng-tools
```

### RHEL/Fedora/CentOS machines

1. Change to the root account;

```
root@netdata~ # su
```

2. And then install;

```
root@netdata~ # yum install rng-tools
```

### After the Installation

You can run the service using the following command;

```
root@netdata~ # service rngd start
```

And also you can check the daemon status using the following command;

```
root@netdata~ # service rngd status
```

</details>



<details><summary>Install Haveged</summary>

Ideally, a system with high entropy demands should have a hardware device to generate random
numbers. For example, a TPM is such a device. However, there are also several software-only options
you may install, like `haveged` [(*read more*)](
https://wiki.archlinux.org/title/Haveged#Installation).

### Installation

### Debian-based platforms

1. To install `haveged`, run:

    ```
    root@netdata~ # sudo apt-get install haveged
    ```

2. Set `haveged` up to start at boot with the command `sudo update-rc.d haveged defaults`.<sup>[3](
   https://www.techrepublic.com/article/how-to-add-more-entropy-to-improve-cryptographic-randomness-on-linux/) </sup>

### RHEL/Fedora/CentOS machines

1. Change to the root account:

    ```
    root@netdata~ # su
    ```

2. Install `haveged`:

    ```
    root@netdata~ # yum install haveged
    ```

3. Set `haveged` to start at boot with the command `chkconfig haveged on`.<sup>[3](
   https://www.techrepublic.com/article/how-to-add-more-entropy-to-improve-cryptographic-randomness-on-linux/) </sup>

</details>
