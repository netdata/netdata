# Getting started Getting started with Netdata Cloud On-Prem Light PoC
Due to the high demand we designed very light and easy to install version of netdata for clients who do not have kubernetes cluster installed. Please keep in mind that this is (for now) only designed to be used as a PoC with no built in resiliency on failures of any kind.

Requirements:
 - Ubuntu 22.04 (clean installation will work best)
 - 10 CPU Cores and 24 GiB of memory
 - Access to shell as a sudo
 - TLS certificate for Netdata Cloud On-Prem PoC. Single endpoint is required. Certificate must be trusted by all entities connecting to the On-Prem installation by any means.

To install whole environment, login to designation host and run:
```shell
curl https://netdata-cloud-netdata-static-content.s3.amazonaws.com/provision.sh
chmod +x provision.sh
sudo ./provision.sh --install
```

What script does?
1. Prompts user to provide:
   - ID and KEY for accessing the AWS (to pull helm charts and container images)
   - License Key
   - URL under which Netdata Cloud Onprem PoC is going to function (without protocol like `https://`)
   - Path for certificate file (unencrypted)
   - Path for private key file (unencrypted)
2. After getting all of the information installation is starting. Script will install:
   1. Helm
   2. Kubectl
   3. AWS CLI
   4. K3s cluster (single node)
3. When all the required software is installed script starts to provision K3s cluster with gathered data.

After cluster provisioning netdata is ready to be used.

#### WARNING
This script will expose automatically expose not only netdata but also a mailcatcher under `<URL from point 1.>/mailcatcher`.
