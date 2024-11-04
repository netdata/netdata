# Installing on hosts without IPv4 connectivity

Our regular installation process requires access to a number of GitHub services that do not have IPv6 connectivity.

As such, using the kickstart install script on such hosts generally does not work, and will typically fail with an error from cURL or wget about connection timeouts.

You can check if your system is affected by this by attempting to connect to (or ping) `https://api.github.com/`. Failing to connect indicates that this issue affects you.

There are three potential workarounds for this:

1. You can configure your system with a proper IPv6 transition mechanism, such as NAT64. GitHub’s anachronisms affect many projects other than just Netdata. There are, unfortunately, a number of other services out there that do not provide IPv6 connectivity, so taking this route is likely to save you time in the future as well.
2. If you are using a system that we publish native packages for (see our [platform support policy](/docs/netdata-agent/versions-and-platforms.md) for more details), you can manually set up our native package repositories as outlined in our [native package install documentation](/packaging/installer/methods/packages.md). Our official package repositories do provide service over IPv6, so they work without issue on hosts without IPv4 connectivity.
3. If neither of the above options work for you, you can still install using our [offline installation instructions](/packaging/installer/methods/offline.md), though do note that the offline install source must be prepared from a system with IPv4 connectivity.
