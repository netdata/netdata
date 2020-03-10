<!--
---
title: "Netdata Documentation"
custom_edit_url: https://github.com/netdata/netdata/edit/master/DOCUMENTATION.md
---
-->

# Netdata Documentation

**Netdata is real-time health monitoring and performance troubleshooting for systems and applications.** It helps you
instantly diagnose slowdowns and anomalies in your infrastructure with thousands of metrics, interactive visualizations,
and insightful health alarms. Plus, long-term storage comes ready out-of-the-box, so can collect, monitor, and maintain
your metrics in one insightful place.

## Navigating the Netdata documentation

Welcome! You've arrived at the documentation for Netdata. Use the links below to find answers to the most common
questions about Netdata, such as how to install it, getting started guides, basic configuration, and adding more charts.
Or, explore all of Netdata's documentation using the table of contents to your left.

<div class="homepage-nav">
  <a class="nav-page" href="packaging/installer/">
    <div class="button-header">
      <h3>Installation guide</h3>
      <svg stroke="currentColor" fill="currentColor" stroke-width="0" viewBox="0 0 448 512" height="1em" width="1em" xmlns="http://www.w3.org/2000/svg"><path d="M224.3 273l-136 136c-9.4 9.4-24.6 9.4-33.9 0l-22.6-22.6c-9.4-9.4-9.4-24.6 0-33.9l96.4-96.4-96.4-96.4c-9.4-9.4-9.4-24.6 0-33.9L54.3 103c9.4-9.4 24.6-9.4 33.9 0l136 136c9.5 9.4 9.5 24.6.1 34zm192-34l-136-136c-9.4-9.4-24.6-9.4-33.9 0l-22.6 22.6c-9.4 9.4-9.4 24.6 0 33.9l96.4 96.4-96.4 96.4c-9.4 9.4-9.4 24.6 0 33.9l22.6 22.6c9.4 9.4 24.6 9.4 33.9 0l136-136c9.4-9.2 9.4-24.4 0-33.8z"></path></svg>
    </div>
    <div class="button-text">
      <p>Use our automated one-line installation script to install Netdata on Linux systems or find detailed instructions for binary packages, Kubernetes, Docker, macOS, and more.</p>
    </div>
  </a>
  <a class="nav-page" href="docs/step-by-step/step-00/">
    <div class="button-header">
      <h3>Step-by-step tutorial</h3>
      <svg stroke="currentColor" fill="currentColor" stroke-width="0" viewBox="0 0 448 512" height="1em" width="1em" xmlns="http://www.w3.org/2000/svg"><path d="M224.3 273l-136 136c-9.4 9.4-24.6 9.4-33.9 0l-22.6-22.6c-9.4-9.4-9.4-24.6 0-33.9l96.4-96.4-96.4-96.4c-9.4-9.4-9.4-24.6 0-33.9L54.3 103c9.4-9.4 24.6-9.4 33.9 0l136 136c9.5 9.4 9.5 24.6.1 34zm192-34l-136-136c-9.4-9.4-24.6-9.4-33.9 0l-22.6 22.6c-9.4 9.4-9.4 24.6 0 33.9l96.4 96.4-96.4 96.4c-9.4 9.4-9.4 24.6 0 33.9l22.6 22.6c9.4 9.4 24.6 9.4 33.9 0l136-136c9.4-9.2 9.4-24.4 0-33.8z"></path></svg>
    </div>
    <div class="button-text">
      <p>Take a guided tour through each of Netdata's core features—perfect for beginners. Follow detailed instructions to monitor your systems and apps, and start your journey into performance troubleshooting.</p>
    </div>
  </a>
  <a class="nav-page" href="docs/getting-started/">
    <div class="button-header">
      <h3>Getting started guide</h3>
      <svg stroke="currentColor" fill="currentColor" stroke-width="0" viewBox="0 0 448 512" height="1em" width="1em" xmlns="http://www.w3.org/2000/svg"><path d="M224.3 273l-136 136c-9.4 9.4-24.6 9.4-33.9 0l-22.6-22.6c-9.4-9.4-9.4-24.6 0-33.9l96.4-96.4-96.4-96.4c-9.4-9.4-9.4-24.6 0-33.9L54.3 103c9.4-9.4 24.6-9.4 33.9 0l136 136c9.5 9.4 9.5 24.6.1 34zm192-34l-136-136c-9.4-9.4-24.6-9.4-33.9 0l-22.6 22.6c-9.4 9.4-9.4 24.6 0 33.9l96.4 96.4-96.4 96.4c-9.4 9.4-9.4 24.6 0 33.9l22.6 22.6c9.4 9.4 24.6 9.4 33.9 0l136-136c9.4-9.2 9.4-24.4 0-33.8z"></path></svg>
    </div>
    <div class="button-text">
      <p>Have some monitoring and system administration experience? Dive right into configuring Netdata, accessing the dashboard, working with the daemon, and changing how Netdata stores metrics.</p>
    </div>
  </a>
  <a class="nav-page" href="docs/configuration-guide/">
    <div class="button-header">
      <h3>Configuration guide</h3>
      <svg stroke="currentColor" fill="currentColor" stroke-width="0" viewBox="0 0 448 512" height="1em" width="1em" xmlns="http://www.w3.org/2000/svg"><path d="M224.3 273l-136 136c-9.4 9.4-24.6 9.4-33.9 0l-22.6-22.6c-9.4-9.4-9.4-24.6 0-33.9l96.4-96.4-96.4-96.4c-9.4-9.4-9.4-24.6 0-33.9L54.3 103c9.4-9.4 24.6-9.4 33.9 0l136 136c9.5 9.4 9.5 24.6.1 34zm192-34l-136-136c-9.4-9.4-24.6-9.4-33.9 0l-22.6 22.6c-9.4 9.4-9.4 24.6 0 33.9l96.4 96.4-96.4 96.4c-9.4 9.4-9.4 24.6 0 33.9l22.6 22.6c9.4 9.4 24.6 9.4 33.9 0l136-136c9.4-9.2 9.4-24.4 0-33.8z"></path></svg>
    </div>
    <div class="button-text">
      <p>Take your configuration options from the <em>getting started guide</em> to the next level. Increase metrics retention, modify how charts are displayed, disable collectors, and modify alarms.</p>
    </div>
  </a>
</div>

**Netdata Cloud**: Use [Netdata Cloud](docs/netdata-cloud/) and the [Nodes View](docs/netdata-cloud/nodes-view.md) to
view real-time, distributed health monitoring and performance troubleshooting data for all your systems in one place.
Add as many nodes as you'd like!

**Advanced users**: For those who already understand how to access a Netdata dashboard and perform basic configuration,
feel free to see what's behind any of these other doors.

-   [Tutorial: Change how long Netdata stores metrics](docs/tutorials/longer-metrics-storage.md): Extend Netdata's
    long-term metrics storage database by allowing Netdata to use more of your system's RAM and disk.
-   [Netdata Behind Nginx](docs/Running-behind-nginx.md): Use an Nginx web server instead of Netdata's built-in server
    to enable TLS, HTTPS, and basic authentication.
-   [Collect more metrics](collectors/README.md) from other services and applications: Enable new internal
    or external plugins and understand when auto-detection works.
-   [Performance](docs/Performance.md): Tips on running Netdata on devices with limited CPU and RAM resources, such as
    embedded devices, IoT, and edge devices.
-   [Streaming](streaming/): Information for those who want to centralize Netdata metrics from any number of distributed
    agents.
-   [Backends](backends/): Learn how to archive Netdata's real-time metrics to a time series database (like Prometheus)
    for long-term archiving.

Visit the [contributing guide](CONTRIBUTING.md), [contributing to documentation
guide](docs/contributing/contributing-documentation.md), and [documentation style
guide](docs/contributing/style-guide.md) to learn more about our community and how you can get started contributing to
Netdata.

**Want to get news, how-tos, and heaps of monitoring savvy straight from Netdata? Subscribe to our newsletter to monitor
our team of monitoring pros.**

<script charset="utf-8" type="text/javascript" src="//js.hsforms.net/forms/shell.js"></script>
<script>
  hbspt.forms.create({
    portalId: "4567453",
    formId: "6a20deb5-a1e6-4312-9c4d-f6862f947fe0"
  });
</script>
