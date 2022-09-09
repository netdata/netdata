<!--
title: "Express installation, deploy Netdata into a linux/unix node (via kickstart)"
custom_edit_url: "https://github.com/netdata/netdata/blob/master/docs/tasks/installation/express-installation-deploy-netdata-into-a-linux-unix-node-(via-kickstart).md"
learn_status: "Published"
learn_topic_type: "Tasks"
learn_rel_path: "installation"
learn_docs_purpose: "Instructions on running the kickstart script on Unix systems."
-->

import { OneLineInstallWget, OneLineInstallCurl } from '../../../src/components/OneLineInstall/'
import Tabs from '@theme/Tabs';
import TabItem from '@theme/TabItem';
import Admonition from '@theme/Admonition';

This page will guide you through installation using the automatic one-line installation script named `kickstart.sh`.

:::info
The kickstart script works on all Linux distributions and, by default, automatic nightly updates are enabled.
:::

## What does `kickstart.sh` do?

The installation script does the following after being downloaded and run using `sh`:

- Determines what platform you are running on.
- Checks for an existing installation, and if found updates that instead of creating a new install.
- Attempts to install Netdata using our [official native binary packages](#native-packages).
- If there are no official native binary packages for your system (or installing that way failed), tries to install
  using a [static build of Netdata](#static-builds) if one is available.
- If no static build is available, installs required dependencies and then attempts to install by
  [building Netdata locally](#local-builds) (by downloading the sources and building them directly).
- Installs `netdata-updater.sh` to `cron.daily`, so your Netdata installation will be updated with new nightly
  versions, unless you override that with an [optional parameter](#optional-parameters-to-alter-your-installation).
- Prints a message whether installation succeeded or failed for QA purposes.

## Prerequisites

- Connection to the internet
- A Linux/UNIX based node
- Either `wget` or `curl` installed on the node

## Steps

Install Netdata by running one of the following options:

<Tabs>
<TabItem value="wget" label=<code>wget</code>>

<OneLineInstallWget/>

</TabItem>
<TabItem value="curl" label=<code>curl</code>>

<OneLineInstallCurl/>

</TabItem>
</Tabs>

If you want to see all the optional parameters to further alter your installation, check
the [kickstart script reference](/packaging/installer/methods/kickstart.md).

### Native packages

We publish official DEB/RPM packages for a number of common Linux distributions as part of our releases and nightly
builds. These packages are available for 64-bit x86 systems. Depending on the distribution and release they may
also be available for 32-bit x86, ARMv7, and AArch64 systems. If a native package is available, it will be used as the
default installation method. This allows you to handle Netdata updates as part of your usual system update procedure.

If you want to enforce the usage of native packages and have the installer return a failure if they are not available,
you can do so by adding `--native-only` to the options you pass to the installer.

### Static builds

We publish pre-built static builds of Netdata for Linux systems. Currently, these are published for 64-bit x86, ARMv7,
AArch64, and POWER8+ hardware. These static builds are able to operate in a mostly self-contained manner and only
require a POSIX compliant shell and a supported init system. These static builds install under `/opt/netdata`. If
you are on a platform which we provide static builds for but do not provide native packages for, a static build
will be used by default for installation.

If you want to enforce the usage of a static build and have the installer return a failure if one is not available,
you can do so by adding `--static-only` to the options you pass to the installer.

### Local builds

For systems which do not have available native packages or static builds, we support building Netdata locally on
the system it will be installed on. When using this approach, the installer will attempt to install any required
dependencies for building Netdata, though this may not always work correctly.

If you want to enforce the usage of a local build (perhaps because you require a custom installation prefix,
which is not supported with native packages or static builds), you can do so by adding `--build-only` to the
options you pass to the installer.

### Verify script integrity

To use `md5sum` to verify the integrity of the `kickstart.sh` script you will download using the one-line command above,
run the following:

```bash
[ "<checksum-will-be-added-in-documentation-processing>" = "$(curl -Ss https://my-netdata.io/kickstart.sh | md5sum | cut -d ' ' -f 1)" ] && echo "OK, VALID" || echo "FAILED, INVALID"
```

If the script is valid, this command will return `OK, VALID`.

## Expected result

The script should exit with a success message.  
To ensure that your installation is working, open up your web browser of choice and navigate to `http://NODE:19999`,
replacing `NODE` with the IP address or hostname of your node.  
If you're interacting with the node locally, and you are unsure of its IP address, try `http://localhost:19999` first.

If the installation was successful, you will be led to the Agent's local dashboard. Enjoy!

## Example

Here we will install Netdata from the stable release channel:

```bash
root@netdata~ # wget -O /tmp/netdata-kickstart.sh https://my-netdata.io/kickstart.sh && sh /tmp/netdata-kickstart.sh --stable-channel
```

## Related topics

1. [Kickstart script reference](/packaging/installer/methods/kickstart.md)