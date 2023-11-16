# Getting started Getting started with Netdata Cloud On-Prem Light PoC
Due to the high demand we designed very light and easy to install version of netdata for clients who do not have kubernetes cluster installed. Please keep in mind that this is (for now) only designed to be used as a PoC with no built in resiliency on failures of any kind.

Requirements:
 - Ubuntu 22.04 (clean installation will work best)
 - 10 CPU Cores and 24 GiB of memory
 - Access to shell as a sudo
 - TLS certificate that is going to be trusted by all agents and web browsers that will use this PoC installation

To install whole environment, login to designation host and run:
```shell
curl <link>
chmod +x provision.sh
sudo ./provision.sh --install
```

What script does?
1. Prompts user to provide:
   - ID and KEY for accessing the AWS (to pull helm charts and container images)
   - License Key
   - URL under which Netdata Cloud Onprem PoC is going to function
   - Path for certificate file (unencrypted)
   - Path for private key file (unencrypted)
2. After getting all of the information installation is starting. Script will install:
   1. Helm
   2. Kubectl
   3. AWS CLI
   4. K3s cluster (single node)
3. When all the required software is installed script starts to provision K3s cluster with gathered data.

After cluster provisioning netdata is ready to be used.