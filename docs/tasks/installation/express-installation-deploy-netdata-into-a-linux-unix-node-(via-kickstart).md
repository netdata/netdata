<!--
title: "Express installation, deploy Netdata into a linux/unix node (via kickstart)"
sidebar_label: "Express installation, deploy Netdata into a linux/unix node (via kickstart)"
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
the [kickstart script reference](https://github.com/netdata/netdata/blob/rework-learn/packaging/installer/methods/kickstart.md)
.

## Further Actions

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

1. [Kickstart script reference](https://github.com/netdata/netdata/blob/rework-learn/packaging/installer/methods/kickstart.md)
