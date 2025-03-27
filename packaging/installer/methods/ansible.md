# Deploy Netdata with Ansible

How do you quickly set up infrastructure monitoring? How can you efficiently deploy Netdata across multiple nodes? How do you make sure the deployment is **reliable, repeatable, and idempotent**? And how can you manage monitoring as **code**?

Meet [Ansible](https://ansible.com), a popular tool for provisioning, configuration management, and infrastructure as code (IaC). It uses **playbooks** to streamline operations with simple syntax, running them securely over SSH—no agent required. That means less setup and more focus on your application and monitoring.

What does **idempotent** mean? 
From the [Ansible glossary](https://docs.ansible.com/ansible/latest/reference_appendices/glossary.html)

> An operation is **idempotent** if running it once produces the same result as running it multiple times, without unintended changes. With Ansible, you can deploy Netdata repeatedly without disrupting your infrastructure—ensuring monitoring as code.

This guide walks you through deploying the **Netdata Agent** across multiple nodes using an [Ansible playbook](https://github.com/netdata/community/tree/main/configuration-management/ansible-quickstart/), managing configurations, and connecting to **Netdata Cloud**—all in minutes.

## Prerequisites

- A Netdata Cloud account. [Sign in and create one](https://app.netdata.cloud) if you don't have one already.
- An administration system with [Ansible](https://www.ansible.com/) installed.
- One or more nodes that your administration system can access via [SSH public
    keys](https://git-scm.com/book/en/v2/Git-on-the-Server-Generating-Your-SSH-Public-Key) (preferably password-less).

## Download and configure the playbook

First, download the
[playbook](https://github.com/netdata/community/tree/main/configuration-management/ansible-quickstart/), move it to the
current directory, and remove the rest of the cloned repository, as it's not required for using the Ansible playbook.

```bash
git clone https://github.com/netdata/community.git
mv community/configuration-management/ansible-quickstart .
rm -rf community
```

Or if you don't want to clone the entire repository, use the [gitzip browser extension](https://gitzip.org/) to get the netdata-agent-deployment directory as a zip file.

Next, `cd` into the Ansible directory.

```bash
cd ansible-quickstart
```

### Edit the `hosts` file

The `hosts` file contains a list of IP addresses or hostnames that Ansible will try to run the playbook against. The
`hosts` file that comes with the repository contains two example IP addresses, which you should replace according to the
IP address/hostname of your nodes.

```text
203.0.113.0  hostname=node-01
203.0.113.1  hostname=node-02 
```

You can also set the `hostname` variable, which appears both on the local Agent dashboard and Netdata Cloud, or you can
omit the `hostname=` string entirely to use the system's default hostname.

#### Set the login user (optional)

If you SSH into your nodes as a user other than `root`, you need to configure `hosts` according to those user names. Use
the `ansible_user` variable to set the login user. For example:

```text
203.0.113.0  hostname=ansible-01  ansible_user=example
```

#### Set your SSH key (optional)

If you use an SSH key other than `~/.ssh/id_rsa` for logging into your nodes, you can set that on a per-node basis in
the `hosts` file with the `ansible_ssh_private_key_file` variable. For example, to log into a Lightsail instance using
two different SSH keys supplied by AWS.

```text
203.0.113.0  hostname=ansible-01  ansible_ssh_private_key_file=~/.ssh/LightsailDefaultKey-us-west-2.pem
203.0.113.1  hostname=ansible-02  ansible_ssh_private_key_file=~/.ssh/LightsailDefaultKey-us-east-1.pem
```

### Edit the `vars/main.yml` file

In order to connect your node(s) to your Space in Netdata Cloud, and see all their metrics in real-time in composite
charts or perform [Metric
Correlations](/docs/metric-correlations.md), you need to set the `claim_token`
and `claim_room` variables.

To find your `claim_token` and `claim_room`, go to Netdata Cloud, then click on your Space's name in the top navigation,
then click on **Manage your Space**. Click on the **Nodes** tab in the panel that appears, which displays a script with
`token` and `room` strings.

![Animated GIF of finding the claiming script and the token and room
strings](https://user-images.githubusercontent.com/1153921/98740235-f4c3ac00-2367-11eb-8ffd-e9ab0f04c463.gif)

Copy those strings into the `claim_token` and `claim_rooms` variables.

```yml
claim_token: XXXXX
claim_rooms: XXXXX
```

Change the `dbengine_multihost_disk_space` if you want to change the metrics retention policy by allocating more or less
disk space for storing metrics. The default is 2048 Mib, or 2 GiB.

Since this node connects to Netdata Cloud, we’ll view its dashboards there instead of using its IP or hostname. The playbook disables the local dashboard by setting `web_mode` to `none`, adding a small security boost by preventing unwanted access.

You can read more about this decision, or other ways you might lock down the local dashboard, in our [node security
doc](/docs/security-and-privacy-design/README.md).

> Curious about why Netdata's dashboard is open by default? Read our [blog
> post](https://www.netdata.cloud/blog/netdata-agent-dashboard/) on that zero-configuration design decision.

## Run the playbook

Time to run the playbook from your administration system:

```bash
ansible-playbook -i hosts tasks/main.yml
```

Ansible first connects to your node(s) via SSH, then [collects
facts](https://docs.ansible.com/ansible/latest/user_guide/playbooks_vars_facts.html#ansible-facts) about the system.
This playbook doesn’t use these facts yet, but you can expand it to set up systems based on your infrastructure.

Next, Ansible makes changes to each node according to the `tasks` defined in the playbook, and
[returns](https://docs.ansible.com/ansible/latest/reference_appendices/common_return_values.html#changed) whether each
task results in a changed, failure, or was skipped entirely.

The task to install Netdata will take a few minutes per node, so be patient! Once the playbook reaches the connect to Cloud
task, your nodes start populating your Space in Netdata Cloud.
