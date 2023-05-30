# Run-time troubleshooting with Functions

Netdata Functions feature allows you to execute on-demand a pre-defined routine on a node where a Netdata Agent is running. These routines are exposed by a given collector. 
These routines can be used to retrieve additional information to help you troubleshoot or to trigger some action to happen on the node itself.


### Prerequisites

The following is required to be able to run Functions from Netdata Cloud.
* At least one of the nodes claimed to your Space should be on a Netdata agent version higher than `v1.37.1`
* Ensure that the node has the collector that exposes the function you want enabled ([see current available functions](https://github.com/netdata/netdata/blob/master/docs/cloud/netdata-functions.md#what-functions-are-currently-available))

### Execute a function (from the Functions tab)

1. From the right-hand bar select the **Function** you want to run
2. Still on the right-hand bar select the **Node** where you want to run it
3. Results will be displayed in the central area for you to interact with
4. Additional filtering capabilities, depending on the function, should be available on right-hand bar

### Execute a function (from the Nodes tab)

1. Click on the functions icon for a node that has this active
2. You are directed to the **Functions** tab
3. Follow the above instructions from step 3.

> ⚠️ If you get an error saying that your node can't execute Functions please check the [prerequisites](#prerequisites).

## Related Topics

### **Related Concepts**
- [Netdata Functions](https://github.com/netdata/netdata/blob/master/docs/cloud/netdata-functions.md)

#### Related References documentation
- [External plugins overview](https://github.com/netdata/netdata/blob/master/collectors/plugins.d/README.md#function)
