<!--
Title: "Express installation, deploy Netdata into a linux/unix node (via kickstart)"
custom_edit_url: https://github.com/netdata/netdata/blob/master/docs/tasks/installation/express-installation,-deploy-netdata-into-a-linux/unix-node-(via-kickstart).md
learn_status: Published
learn_topic_type: Tasks
learn_rel_path: docs/tasks/installation/express-installation,-deploy-netdata-into-a-linux/unix-node-(via-kickstart).md

learn_docs_purpose: Instructions on running the kickstart script on Unix systems.
-->

import { OneLineInstallWget, OneLineInstallCurl } from '../../../src/components/OneLineInstall/'

This page will guide you through installation using the automatic one-line installation script named `kickstart.sh`.

:::info
The kickstart script works on all Linux distributions and, by default, automatic nightly updates are enabled.
:::

Read more about the kickstart script and it's options in the [related topics section](#related-topics).

## Prerequisites

- Connection to the internet
- A Linux based node
- Either `wget` or `curl` installed on the node  

## Steps

To install Netdata, run the following:

<OneLineInstallWget/>

Or, if you have cURL but not wget:

<OneLineInstallCurl/>

## Expected result

The script should exit with a success message.  
To ensure that your installation is working, open up your web browser of choice and navigate to `http://NODE:19999`, replacing `NODE` with the IP address or hostname of your node.  
If you're interacting with the node locally and you are unsure of it's IP address, try `http://localhost:19999` first.

If the installation was successful, you will be led to the Agent's local dashboard. Enjoy!

## Example

Here we will install Netdata from the stable release channel:

```bash
root@netdata~ # wget -O /tmp/netdata-kickstart.sh https://my-netdata.io/kickstart.sh && sh /tmp/netdata-kickstart.sh --stable-channel
```

## Related topics

[kickstart reference page](ADD LINK)
