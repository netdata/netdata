# Deploy and Identify Fleet Devices

Start with a capability inventory for each hardware and workload class. A signage player may need process metrics and display-service monitoring; a robot may also need hardware sensors, container attribution and custom application telemetry. Give each class a versioned package policy and runtime configuration policy.

## Choose compatible packages

For Linux static packages, choose `armv6l` or `armv7l` according to the device CPU, ABI and operating system. ARMv7 hardware does not automatically make every ARMv7 package compatible with its userspace. Test startup, collection and streaming on the actual image. This workflow also accepts supported static packages for other architectures; it does not convert binaries or add unsupported OS support.

Use the [static installation guide](../../packaging/makeself/README.md) to understand the installation method. Run package reduction on a build host, before installing into a clean image. Do not run it against a device's live `/opt/netdata` directory.

## Give every device a unique identity

Do not clone a commissioned Agent's machine identity or Cloud credentials into a fleet image. Generate or enroll identity on first boot, using the [VM template guidance](../learn/vm-templates.md) as the identity reference. Preserve the identity across normal updates; deliberate reprovisioning needs an explicit identity policy.

Use stable hostnames and host labels for operational grouping: device class, site, deployment ring, hardware revision and software release. Keep secrets and personal information out of labels. See [host labels](../netdata-agent/configuration/organize-systems-metrics-and-alerts.md) for the supported configuration.

## Configure streaming securely

Configure the Child's `[stream]` section in `stream.conf` with streaming enabled, a Parent destination and an API key. Configure the corresponding API-key receiver section on each Parent. Streaming authorization keys and Cloud claiming credentials serve different purposes.

Use encrypted streaming with certificate verification for untrusted networks. Provision the required CA chain and Parent server certificate; do not solve certificate failures by disabling verification. Streaming uses its own protocol, so an ordinary HTTP proxy is insufficient. See the [Parent-Child configuration reference](../../src/streaming/README.md) for destination syntax, TLS, authorization and allowed source addresses.

Allow only the intended fleet sources on the Parent. If devices use changing cellular addresses, use an appropriate network access design and securely provisioned credentials rather than assuming a fixed source address.

## Commission each image class

Before rollout, confirm:

- Each new device appears as a distinct Child, with the expected labels and release.
- Required contexts, dimensions and Functions reach the Parent.
- Alerts and anomaly detection run on the Parent according to the fleet policy.
- Local web exposure, custom collectors and plugin privileges match the deployment policy.
- The [package manifest](./minimize-disk-footprint.md) matches the intended capabilities.
- Device CPU, RAM, writes and network traffic meet measured budgets during normal operation, commissioning and reconnects.

Keep a representative device in each deployment ring. Different kernels, peripherals and service versions can produce different metric counts and resource costs even with identical Netdata configuration.
