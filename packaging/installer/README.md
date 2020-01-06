# Installation guide

Netdata is a monitoring agent designed to run on all your systems: physical and virtual servers, containers, even
IoT/edge devices. Netdata runs on Linux, FreeBSD, macOS, Kubernetes, Docker, and all their derivatives.

The best way to install Netdata is with our [**automatic one-line installation
script**](#automatic-one-line-installation-script), which works with all Linux distributions. To see other installation
methods, such as those for different operating systems including alternatives to the one-line installer script, see
[Have a different operating system, or want to try another
method?](#have-a-different-operating-system-or-want-to-try-another-method)

Some third parties, such as the packaging teams at various Linux distributions, distribute old, broken, or altered
packages. We recommend you install Netdata using one of the above methods to guarantee you get the latest and
checksum-verified packages.

Starting with v1.12, Netdata collects anonymous usage information by default and sends it to Google Analytics. Read
about the information collected, and learn how to-opt, on our [anonymous statistics](../../docs/anonymous-statistics.md)
page.

The usage statistics are _vital_ for us, as we use them to discover bugs and priortize new features. We thank you for
_actively_ contributing to Netdata's future.

## Automatic one-line installation script

![](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.requests_per_url&options=unaligned&dimensions=kickstart&group=sum&after=-3600&label=last+hour&units=installations&value_color=orange&precision=0) ![](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.requests_per_url&options=unaligned&dimensions=kickstart&group=sum&after=-86400&label=today&units=installations&precision=0)

This method is fully automatic on all Linux distributions. To install Netdata from source and get _automatic nightly
updates_, run the following:

```bash
bash <(curl -Ss https://my-netdata.io/kickstart.sh)
```

> Do not use `sudo`—the installer will escalate privileges if needed.

Now that Netdata is installed, be sure to visit our [getting started guide](../../docs/getting-started.md) for a quick
overview of configuring Netdata, enabling plugins, and controlling Netdata's daemon. Or, get the full guided tour of
Netdata's capabilities with our [step-by-step tutorial](../../docs/step-by-step/step-00.md)!

## Have a different operating system, or want to try another method?



## Automatic updates

By default, Netdata's installation scripts enable automatic updates for both nightly and stable release channels.

If you would prefer to manually update your Netdata agent, you can disable automatic updates by using the `--no-updates` option when you install or update Netdata using the [one-line installation script](#one-line-installation).

```bash
# kickstart.sh
bash <(curl -Ss https://my-netdata.io/kickstart.sh) --no-updates

# kickstart-static64.sh
bash <(curl -Ss https://my-netdata.io/kickstart-static64.sh) --no-updates
```

With automatic updates disabled, you can choose exactly when and how you [update Netdata](UPDATE.md).

## Nightly vs. stable releases

The Netdata team maintains two releases of the Netdata agent: **nightly** and **stable**. By default, Netdata's
installation scripts will give you **automatic, nightly** updates, as that is our recommended configuration.

**Nightly**: We create nightly builds every 24 hours. They contain fully-tested code that fixes bugs or security flaws,
or introduces new features to Netdata. Every nightly release is a candidate for then becoming a stable release—when
we're ready, we simply change the release tags on GitHub. That means nightly releases are stable and proven to function
correctly in the vast majority of Netdata use cases. That's why nightly is the _best choice for most Netdata users_.

**Stable**: We create stable releases whenever we believe the code has reached a major milestone. Most often, stable
releases correlate with the introduction of new, significant features. Stable releases might be a better choice for
those who run Netdata in _mission-critical production systems_, as updates will come more infrequently, and only after
the community helps fix any bugs that might have been introduced in previous releases.

**Pros of using nightly releases:**

  - Get the latest features and bug fixes as soon as they're available
  - Receive security-related fixes immediately
  - Use stable, fully-tested code that's always improving
  - Leverage the same Netdata experience our community is using

**Pros of using stable releases:**

-   Protect yourself from the rare instance when major bugs slip through our testing and negatively affect a Netdata
    installation
-   Retain more control over the Netdata version you use
