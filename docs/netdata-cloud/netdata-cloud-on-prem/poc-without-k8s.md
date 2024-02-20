# Netdata Cloud On-Prem PoC without k8s

These instructions are about installing a light version of Netdata Cloud, for clients who do not have a Kubernetes cluster installed. This setup is **only for demonstration purposes**, as it has no built-in resiliency on failures of any kind.

## Requirements

- Ubuntu 22.04 (clean installation will work best).
- 10 CPU Cores and 24 GiB of memory.
- Access to shell as a sudo.
- TLS certificate for Netdata Cloud On-Prem PoC. A single endpoint is required. The certificate must be trusted by all entities connecting to this installation.
- AWS ID and License Key - we should have provided this to you, if not contact us: <info@netdata.cloud>.

To install the whole environment, log in to the designated host and run:

```bash
curl https://netdata-cloud-netdata-static-content.s3.amazonaws.com/provision.sh -o provision.sh
chmod +x provision.sh
sudo ./provision.sh install \
      -key-id "" \
      -access-key "" \
      -onprem-license-key "" \
      -onprem-license-subject "" \
      -onprem-url "" \
      -certificate-path "" \
      -private-key-path ""
```

What does the script do during installation?

1. Prompts for user to provide:
    - `-key-id` - AWS ECR access key ID.
    - `-access-key` - AWS ECR Access Key.
    - `-onprem-license-key` - Netdata Cloud On-Prem license key.
    - `-onprem-license-subject` - Netdata Cloud On-Prem license subject.
    - `-onprem-url` - URL for the On-prem (without http(s) protocol).
    - `-certificate-path` - path to your PEM encoded certificate.
    - `-private-key-path` - path to your PEM encoded key.

2. After all the above installation will begin. The script will install:
    - Helm
    - Kubectl
    - AWS CLI
    - K3s cluster (single node)

3. When all the required software is installed script starts to provision the K3s cluster with gathered data.

After cluster provisioning netdata is ready to be used.

> WARNING:
> This script will automatically expose not only netdata but also a mailcatcher under `<URL from point 1.>/mailcatcher`.

## How to log in?

Only login by mail can work without further configuration. Every mail this Netdata Cloud On-Prem sends, will appear on the mailcatcher, which acts as the SMTP server with a simple GUI to read the mails.

Steps:

1. Open Netdata Cloud On-Prem PoC in the web browser on URL you specified
2. Provide email and use the button to confirm
3. Mailcatcher will catch all the emails so go to `<URL from point 1.>/mailcatcher`. Find yours and click the link.
4. You are now logged into Netdata Cloud. Add your first nodes!

## How to remove Netdata Cloud On-Prem PoC?

To uninstall the whole PoC, use the same script that installed it, with the `uninstall` switch.

```shell
cd <script dir>
sudo ./provision.sh uninstall
```
