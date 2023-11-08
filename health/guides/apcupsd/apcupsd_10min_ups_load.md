### Understand the alert

This alert is related to your APC uninterruptible power supply (UPS) device. If you receive this alert, it means that your UPS is experiencing high load, which could result in it entering bypass mode or shutdown to protect the device. The alert is triggered in a warning state when the average UPS load is between 70-80% and in a critical state when it is between 85-95%.

### Troubleshoot the alert

Follow these steps to address the high load on your UPS device:

1. **Identify devices connected to the UPS**: Make a list of all the devices connected to the UPS. This list could include computers, servers, routers, and other essential equipment.

2. **Assess the importance of each device**: Prioritize the devices connected to the UPS based on their importance to your network infrastructure. Determine which devices are mission-critical and which ones can be temporarily disconnected without causing significant disruptions.

3. **Disconnect non-critical devices**: Once you have assessed the importance of the connected devices, disconnect any non-critical devices to reduce the load on the UPS. This action will help ensure that the mission-critical devices continue to receive power during a utility failure.

4. **Consider additional UPS capacity**: If you frequently receive this alert or are unable to disconnect enough devices to reduce the load on the UPS, consider adding additional UPS capacity to your infrastructure. This additional capacity could come in the form of additional UPS units or a larger UPS with a higher output capacity.

5. **Monitor the UPS load**: After disconnecting non-critical devices or adding additional UPS capacity, continue monitoring the UPS load using the Netdata Agent to ensure the load stays within acceptable limits. If the alert persists, you may need to reevaluate your infrastructure and device connections.

### Useful resources

1. [APC UPS Management](https://www.schneider-electric.com/en/product-category/870_IDSof_0145_NET/!ut/p/z1/hZBNbsIwDMD3ejK_Sh4xWb1tgwEfkFLCVKrUYKngigoXrWtJ_gSCk_bm0RfbT707TIAx8WuuDIwdwmK28Q2YY3Agq3XkKAGwpTEgZUPAHD7HxAqcAkgxV7OuBHSkrBSV7eGzvdN1jQZSYhNnhP7YvfFGttb8j7LlPvTXSuC7V-q1DXce8XtWjZmfrniT7ufcTtT8AKaWHzA!!/dz/d5/L2dBISEvZ0FBIS9nQSEh/)
2. [Understanding the Different Types of UPS Systems](https://www.apc.com/us/en/faqs/FA157448/)
