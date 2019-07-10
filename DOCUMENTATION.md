# Netdata Documentation

**Netdata is real-time health monitoring and performance troubleshooting for systems and applications.** It helps you instantly diagnose slowdowns and anomalies in your infrastructure with thousands of metrics, interactive visualizations, and insightful health alarms.


## Navigating the Netdata documentation

Welcome! You've arrived at the documentation for Netdata. Use the links below to find answers to the most common questions about Netdata, such as how to install it, getting started guides, basic configuration, and adding more charts. Or, explore all of Netdata's documentation using the table of contents to your left.
 
1. **[Installation](packaging/installer/#installation)**: Step-by-step instructions on how to install Netdata either using our quick-start installer on Linux systems or manual installations on a variety of alternative systems. **[Go &rarr;](packaging/installer/#installation)**

2. **[Getting Started Guide](docs/GettingStarted.md)**: The perfect place for Netdata beginners to start. Learn how to access Netdata's dashboard, start and stop the service, basic configuration, and more.  **[Go &rarr;](docs/GettingStarted.md)**

3. **[Configuration Guide](docs/configuration-guide.md)**: Take your configuration options from the *getting started guide* to the next level. Increase metrics retention, modify how charts are displayed, disable collectors, and modify alarms.  **[Go &rarr;](docs/configuration-guide.md)**

4. **Advanced users**: For those who already understand how to access a Netdata dashboard and perform basic configuration, feel free to see what's behind any of these other doors.

    - [Netdata Behind Nginx](docs/Running-behind-nginx.md): Use an Nginx web server instead of Netdata's built-in server to enabled TLS/SSL and basic authentication.
    - [Add More Charts](docs/Add-more-charts-to-netdata.md): Enable new internal or external plugins and understand when auto-detection works.
    - [Performance](docs/Performance.md): Tips on running Netdata on devices with limited CPU and RAM resources, such as embedded devices, IoT, and edge devices.
    - [Streaming](streaming/): Information for those who want to centralize Netdata metrics from any number of distributed agents.
    - [Backends](backends/): Learn how to archive Netdata's real-time metrics to a time series database (like Prometheus) for long-term archiving.


Visit the [contributing](CONTRIBUTING.md) page to find guides about the Netdata code of conduct, our community, and how you can get started contributing to Netdata.


## Subscribe for news and tips from monitoring pros

<script charset="utf-8" type="text/javascript" src="//js.hsforms.net/forms/shell.js"></script>
<script>
  hbspt.forms.create({
    portalId: "4567453",
    formId: "6a20deb5-a1e6-4312-9c4d-f6862f947fe0"
});
</script>

---

![A GIF of the standard Netdata dashboard](https://user-images.githubusercontent.com/2662304/48346998-96cf3180-e685-11e8-9f4e-059d23aa3aa5.gif)