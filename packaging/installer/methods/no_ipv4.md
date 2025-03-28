# Installing on hosts without IPv4 connectivity


Our regular installation process requires access to GitHub services that lack IPv6 connectivity. Using the kickstart install script on such hosts may result in connection timeouts from cURL or wget.

To check if your system is affected, try connecting to or pinging `https://api.github.com/`. If it fails, the issue impacts you.
There are three potential workarounds for this:

1. You can configure your system with an IPv6 transition mechanism like NAT64. GitHub’s lack of IPv6 affects many projects, not just Netdata, and other services may have similar issues. Setting this up can save you time in the future.
2. If you are using a system that we publish native packages for (see our [platform support policy](/docs/netdata-agent/versions-and-platforms.md) for more details), you can manually set up our native package repositories as outlined in our [native package install documentation](/packaging/installer/methods/packages.md). Our official package repositories do provide service over IPv6, so they work without issue on hosts without IPv4 connectivity.
3. If neither of the above options work for you, you can still install using our [offline installation instructions](/packaging/installer/methods/offline.md), though do note that the offline install source must be prepared from a system with IPv4 connectivity.