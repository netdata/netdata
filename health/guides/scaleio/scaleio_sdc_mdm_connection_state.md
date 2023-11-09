### Understand the alert

The `scaleio_sdc_mdm_connection_state` alert indicates that your ScaleIO Data Client (SDC) is disconnected from the ScaleIO MetaData Manager (MDM). This disconnection can lead to potential performance issues or data unavailability in your storage infrastructure.

### Troubleshoot the alert

1. Check the connectivity between SDC and MDM nodes.

Verify that the SDC and MDM nodes are reachable by performing a `ping` or using `traceroute` from the SDC node to the MDM node and vice versa. Network connectivity issues such as high latency or packet loss may cause the disconnection between SDC and MDM.

2. Examine log files.

Review the SDC and MDM log files to identify any error messages or warnings that can indicate the reason for the disconnection. Common log file locations are:

   - SDC logs: `/opt/emc/scaleio/sdc/logs/sdc.log`
   - MDM logs: `/opt/emc/scaleio/mdm/logs/mdm.log`

3. Check the status of ScaleIO services.

Verify that the ScaleIO services are running on both the SDC and MDM nodes. You can check the service status with the following commands:

   - SDC service status: `sudo systemctl status scaleio-sdc`
   - MDM service status: `sudo systemctl status scaleio-mdm`

If any of the services are not running, start them and check the connection state again.

4. Reconnect SDC to MDM.

If the issue still persists after verifying the network connectivity and services' statuses, try to reconnect the SDC to MDM manually. Use the following command on the SDC node:

   ```
   sudo scli --reconnect_sdc --mdm_ip <MDM_IP_ADDRESS>
   ```

Replace `<MDM_IP_ADDRESS>` with the IP address of your MDM node.

5. Contact support.

If the disconnection issue persists after trying the above steps, consider contacting technical support for assistance.

### Useful resources

1. [ScaleIO Troubleshooting](https://www.dell.com/support/home/en-us/product-support/product/scaleio)
