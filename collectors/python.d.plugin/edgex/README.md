
# EdgeX 

## EdgeX is an Open Source IoT Platform, hosted by the Linux Foundation 


**As mentioned in their site:**

EdgeX FoundryTM is a vendor-neutral open source project hosted by The Linux Foundation building a common open framework for IoT edge computing.  At the heart of the project is an interoperability framework hosted within a full hardware- and OS-agnostic reference software platform to enable an ecosystem of plug-and-play components that unifies the marketplace and accelerates the deployment of IoT solutions.

EdgeX is an important enabler for interested parties to freely collaborate on open and interoperable IoT solutions built using existing connectivity standards combined with their own proprietary innovations.



**Website:** [EdgeX Foundry](https://www.edgexfoundry.org/)

![](https://www.edgexfoundry.org/wp-content/uploads/sites/25/2018/09/EdgeX_PlatformArchitectureDiagram-1024x651.png)

**Metrics:**

As of version 0.2:

- Readings/second
- Events/second
- Absolute number of Events/Readings
- Number of registered devices
- Memory Metrics for core services (edgex-core-data, edgex-core-metadata, edgex-core-command, edgex-support-logging). These metrics are generated using the Golang [runtime Package](https://golang.org/pkg/runtime/#ReadMemStats).

        - Alloc - currently allocated number of bytes on the heap,
        - Mallocs and Frees - number of allocations, deallocations
        - Live objects (mallocs - frees)
