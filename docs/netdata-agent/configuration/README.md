# Netdata Agent Configuration

:::info

You can configure Netdata Agents on the fly using Netdata Cloud. Check the [Dynamic Configuration Manager](/docs/netdata-agent/configuration/dynamic-configuration.md) documentation for details.

:::

You configure your Netdata Agent using the main configuration file `netdata.conf`. This guide shows you how to locate, edit, and manage this configuration file.

## Locate Your Config Directory

First, you need to find where your configuration files are stored. On most Linux systems, you'll find your **Netdata config directory** at `/etc/netdata/`. This directory contains:

- Several configuration files with the `.conf` extension
- A few directories for specific configurations  
- A shell script named `edit-config` for safely editing files

:::tip 

Some operating systems use `/opt/netdata/etc/netdata/` as the config directory. 
If you're **not sure where yours is located**, navigate to `http://NODE:19999/netdata.conf` in your browser (replace `NODE` with your node's IP address or hostname) and find the `# config directory =` setting. The value listed shows your system's config directory.

:::

:::note 

All of Netdata's documentation **assumes your config directory is at** `/etc/netdata`, and that you run any scripts from inside that directory.

:::

## Edit Configuration Files

<details>
<summary><strong>Method 1: Using `edit-config` (Recommended)</strong></summary><br/>

You should use the `edit-config` script for making configuration changes. This script lives inside your config directory and helps you manage and safely edit configuration files.

To edit `netdata.conf`:

1. Navigate to your config directory and run the edit script:

```bash
cd /etc/netdata 2>/dev/null || cd /opt/netdata/etc/netdata
sudo ./edit-config netdata.conf
```

2. Your default editor will open with the configuration file
3. Make your changes and save the file

</details>

<details>
<summary><strong>Method 2: Download Current Configuration</strong></summary><br/>

If you want to work with the exact configuration currently running on your Agent, you can download it directly.

You can download the running version of `netdata.conf` from your running Netdata Agent at this URL:

```url
http://agent-ip:19999/netdata.conf
```

To download and replace your current configuration file:

```bash
cd /etc/netdata 2>/dev/null || cd /opt/netdata/etc/netdata
curl -ksSLo /tmp/netdata.conf.new http://localhost:19999/netdata.conf && sudo mv -i /tmp/netdata.conf.new netdata.conf 
```

This method is useful when you want to:
- Backup your current running configuration
- Start with the default settings that are currently active
- Replicate configuration across multiple agents

</details>