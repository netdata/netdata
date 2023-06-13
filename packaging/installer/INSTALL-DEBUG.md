# Troubleshoot problems during Agent installation and known issues

#### Older distributions (Ubuntu 14.04, Debian 8, CentOS 6) and OpenSSL

If you're running an older Linux distribution or one that has reached EOL, such as Ubuntu 14.04 LTS, Debian 8, or CentOS
6, your Agent may not be able to securely connect to Netdata Cloud due to an outdated version of OpenSSL. These old
versions of OpenSSL cannot perform [hostname validation](https://wiki.openssl.org/index.php/Hostname_validation), which
helps securely encrypt SSL connections.

If you choose to continue using the outdated version of OpenSSL, your node will still connect to Netdata Cloud, albeit
with hostname verification disabled. Without verification, your Netdata Cloud connection could be vulnerable to
man-in-the-middle attacks.

#### Known issues with the yum package manager

In most of the RPM distros that use yum package manager, Our kickstart script is trying to set up our private repo
to ship the packages and install the native packages of Netdata. Open-source package maintainers also ship Netdata Agent
on their distro repos. To instruct the package manager to fetch packages from our repos, we rely upon a concept called
priorities. The yum package manager needs the `yum-plugin-priorities` package for the package manager to start
respecting priority rules. Consider installing this plugin, before running the kickstart script.

```sh
sudo yum install yum-plugin-priorities
```

#### CentOS 6 and CentOS 8

To install the Agent on certain CentOS and RHEL systems, you must enable non-default repositories, such as EPEL or
PowerTools, to gather hard dependencies. See
the [CentOS 6](https://github.com/netdata/netdata/blob/master/packaging/installer/methods/manual.md#centos--rhel-6x) and
[CentOS 8](https://github.com/netdata/netdata/blob/master/packaging/installer/methods/manual.md#centos--rhel-8x)
sections for more information.

#### Access to file is not permitted

If you see an error similar to `Access to the file is not permitted: /usr/share/netdata/web//index.html` when you try to
visit the Agent dashboard at `http://NODE:19999`, you need to update Netdata's permissions to match those of your
system.

Run `ls -la /usr/share/netdata/web/index.html` to find the file's permissions. You may need to change this path based on
the error you're seeing in your browser. In the below example, the file is owned by the user `root` and the group
`root`.

```bash
ls -la /usr/share/netdata/web/index.html
-rw-r--r--. 1 root root 89377 May  5 06:30 /usr/share/netdata/web/index.html
```

These files need to have the same user and group used to install your Netdata. Suppose you installed Netdata with user
`netdata` and group `netdata`, in this scenario you will need to run the following command to fix the error:

```bash
# chown -R netdata.netdata /usr/share/netdata/web
```

## Unknown install-type (F050C)

The unknown install type is a Netdata Agent deployment we can be certain how it was installed. To that end,
we don't attempt to make any automated/scripted changes to not affect your system adversely.

### Which type of Netdata deployments are characterized as "Unknown"

In a nutshell, whatever package/binary/deployment was not shipped by the Netdata team. This category includes:

- Native packages shipped by community repos.
- Custom docker files (from private forks)

### Troubleshoot

#### Native deployments from open-source package maintainers

In cases like these, we can't guarantee that everything will work as expected or that the integrations with other
Netdata products (such as the Netdata Cloud) will be seamless. If you choose to receive the Netdata Agent from
your distribution official repos, you need to disable any auto-update feature of the Netdata Agent (for
instance from the cron scheduler) and rely on your package manager (manual or automated updates).

Otherwise, you need to remove the packages installed (via your package manager) and follow
our [official instructions](https://learn.netdata.cloud/docs/install-the-netdata-agent/) to deploy the Agent.

## Native packages fail to be installed, Netdata repo conflict

We ship Netdata Agent via our private repo `https://repo.netdata.cloud/repos/`. In this repo, we ship two different
channels of packages ([Nightly & Stable](https://github.com/netdata/netdata/edit/master/packaging/installer/README.md#nightly-vs-stable-releases))
which are mutually excluded. In a nutshell, when you peak the version of the Agents which serve your needs, you only
receive updates on this particular channel. In cases when you decided that a Chanel doesn't serve your needs and you want
to change, you need to manually delete the repo you track at the moment from your package manager.
