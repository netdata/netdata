# Dynamic Configuration Manager

## Table of Contents

- [Overview](#overview)
- [Quick Access Methods](#quick-access-methods)
- [Getting Started](#getting-started)
- [Collectors](#collectors)
- [Multi-Node Deployment](#multi-node-deployment)

## Overview

:::important

Netdata Cloud paid subscription is required.

:::

:::tip

**What You'll Learn**

How to access the Dynamic Configuration Manager and understand its key features for managing your monitoring infrastructure.

:::

The Dynamic Configuration Manager allows you to configure collectors and alerts directly through the Netdata UI. This feature enables you to:

- **Create, test, and deploy configurations** for one or more nodes directly within the UI
- **Eliminate manual command-line edits and node access**, enhancing your workflow efficiency

:::note

**Cloud Connection and Security**

Your nodes using Dynamic Configuration Manager require a connection to Netdata Cloud. This ensures proper permission handling and data security.

:::

**Key Features:**

| Feature                                  | Purpose                                                                                       |
|------------------------------------------|-----------------------------------------------------------------------------------------------|
| Configure collector parameters           | Set up data collection settings directly in the UI                                            |
| Fill out configuration forms             | Use guided interfaces instead of editing config files                                         |
| Test configurations before deployment    | Validate settings to prevent errors                                                           |
| Create alert templates                   | Build reusable alert definitions                                                              |
| Apply templates to instances or contexts | Target specific services or apply broadly                                                     |
| Deploy to multiple nodes simultaneously  | Ensure consistency across your infrastructure                                                 |
| Manage Health tab alert templates        | Create configurations for templates and individual alerts that apply to instances or contexts |

:::info

To understand what actions you can perform based on your role, refer to the [Role-Based Access documentation](/docs/netdata-cloud/authentication-and-authorization/role-based-access-model.md).

:::

## Quick Access Methods

:::tip

**What You'll Learn**

Four different ways to access the Dynamic Configuration Manager, each optimized for different workflows.

:::

You can access the Dynamic Configuration Manager in multiple ways:

<details>
<summary><strong>From Any Chart</strong></summary><br/>

1. Navigate to any chart on your dashboard
2. Click the **Alert icon (bell icon)** at the top of the chart
3. Choose to edit an existing alert or create a new one
4. Configure your alert parameters and submit changes

<br/>
</details>

<details>
<summary><strong>From the Alerts Tab</strong></summary><br/>

1. Go to the **Alerts tab** on your Netdata dashboard
2. Locate the alert you want to modify and click on it
3. Adjust thresholds and parameters to match your needs
4. Save your changes

<br/>
</details>

<details>
<summary><strong>From the Integrations Section</strong></summary><br/>

1. Navigate to the **Integrations section** on your dashboard
2. Browse through the available collectors
3. Click on the **Configure** button for the collector you want to set up
4. Once configured, they will start collecting data as specified

<br/>
</details>

<details>
<summary><strong>From Space Settings</strong></summary><br/>

1. Go to **Space Settings** on your Dashboard
2. Navigate to the **Configurations** section
3. Explore, create, and edit collector, health and logs configurations

<br/>
</details><br/>

:::tip

Currently available for go.d collectors, you can configure collectors straight from the Integrations section. This means you can quickly identify what Netdata can monitor and set up your configurations in one go.

:::

## Getting Started

:::tip

**What You'll Learn**

How to use each of the four access methods with practical step-by-step workflows.

:::

:::tip

To help you get started with the Dynamic Configuration Manager, try using the Netdata demo environment to explore these capabilities firsthand and see how they can enhance your monitoring workflows.

:::

### Step-by-Step Walkthrough

<details>
<summary><strong>Creating Alerts from Charts</strong></summary><br/>

Learn more about this access method: [From Any Chart](#quick-access-methods)

1. Navigate to the chart (context) you want to create an alert for
2. Click on the Alert icon (bell icon) on top of the chart to edit an existing alert or create a new one
3. Configure your alert parameters, such as rules, instances, thresholds, etc.
4. Submit your changes

<br/>
</details>

<details>
<summary><strong>Managing Alerts from the Alerts Tab</strong></summary><br/>

Learn more about this access method: [From the Alerts Tab](#quick-access-methods)

1. Go to the Alerts tab on your Netdata dashboard
2. Locate the alert you wish to modify and click on it
3. Adjust the thresholds and other parameters to match your specific needs
4. Save the changes

<br/>
</details>

<details>
<summary><strong>Configuring Collectors from Integrations</strong></summary><br/>

Learn more about this access method: [From the Integrations Section](#quick-access-methods)

1. Navigate to the Integrations section on the dashboard
2. Browse through the available collectors
3. Click on the **Configure** button for the collector you want to set up
4. Once configured, they will start collecting data as specified

<br/>
</details>

<details>
<summary><strong>Managing Configurations from Space Settings</strong></summary><br/>

Learn more about this access method: [From Space Settings](#quick-access-methods)

1. Go to **Space Settings** on your Dashboard
2. Navigate to the **Configurations** section
3. Explore, create, and edit collector, health and logs configurations

<br/>
</details>

## Collectors

:::tip

**What You'll Learn**

How modules and jobs work together to collect data, and the specific actions you can perform on each.

:::

### Module

A module represents a specific data collector, such as Apache, MySQL, or Redis. Think of modules as templates for data collection.

Each module can have multiple jobs, which are unique configurations of that template tailored to your specific needs.

**Module Management Actions:**

| Action             | Description                                                                                                               |
|--------------------|---------------------------------------------------------------------------------------------------------------------------|
| **Add job**        | Create new configuration instances (jobs) for a particular module                                                         |
| **Enable/Disable** | Disabling a module deactivates all currently running jobs and prevents any future jobs from being created for that module |

### Job

A job represents a running instance of a module with a specific configuration. Think of it as a customized data collection task based on a module template.

**Job Source Types:**

Every job has a designated "source type" indicating its origin:

| Source Type               | Description                                                                       |
|---------------------------|-----------------------------------------------------------------------------------|
| **Stock**                 | Pre-installed with Netdata and provides basic data collection for common services |
| **User**                  | Created from user-defined configuration files on the node                         |
| **Discovered**            | Automatically generated by Netdata upon discovering a service running on the node |
| **Dynamic Configuration** | Managed and created through the Dynamic Configuration Manager                     |

**Job Management Actions:**

| Category          | Action             | Description                                                                                                                                                                                |
|-------------------|--------------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| **Configuration** | **Edit**           | Modify an existing job's configuration                                                                                                                                                     |
| **Configuration** | **Test**           | Validate newly created or edited configurations before applying them permanently                                                                                                           |
| **Management**    | **Enable/Disable** | Control the job's activity. Disabling a running job stops data collection                                                                                                                  |
| **Management**    | **Restart**        | Restart a job's data collection, useful if a job encounters a "Failed" state. Upon restart, you'll see a notification with the failure message                                             |
| **Management**    | **Remove**         | Delete a job configuration entirely. Note that you can only remove jobs created through Dynamic Configuration. Other job types originate from files on the node and cannot be deleted here |

:::important

Only jobs created through Dynamic Configuration can be removed. Other job types originate from files on the node and can’t be deleted through the UI.

:::

## Multi-Node Deployment

:::tip

**What You'll Learn**

How to deploy configurations to multiple nodes simultaneously, saving time and ensuring consistency across your infrastructure.

:::

The Dynamic Configuration Manager allows you to submit configurations to multiple nodes with just one click, eliminating the need to configure each node individually.

:::note
For teams using Infrastructure as Code solutions, the Dynamic Configuration Manager allows you to construct and copy configurations easily, integrating them into your IaC workflows. This ensures your configurations are consistent and reproducible across different environments.
:::

### Multi-Node Deployment Process

<details>
<summary><strong>Deploy to Multiple Nodes</strong></summary><br/>

1. Configure your collectors or alerts using any of the methods described above
2. Use the multi-node feature to select your target nodes
3. Submit your configuration, and it will be applied to all selected nodes instantly

<br/>
</details><br/>

:::note

This feature is particularly valuable for managing large infrastructures where manual configuration of individual nodes would be time-consuming and error-prone.

:::

Experience the efficiency and power of the Dynamic Configuration Manager in Netdata today. Whether you're managing a handful of nodes or a vast infrastructure, this feature will make your monitoring and alerting tasks smoother and more intuitive.

Developing with dynamic configuration? [Click here](https://learn.netdata.cloud/docs/developer-and-contributor-corner/dynamic-configuration/).
