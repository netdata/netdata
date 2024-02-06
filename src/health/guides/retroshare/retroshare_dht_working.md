### Understand the alert

The `retroshare_dht_working` alert is related to the Retroshare service, which is a secure communication and file sharing platform. Retroshare uses a Distributed Hash Table (DHT) to manage the network of connected users.

If you receive this alert, it means that the number of DHT peers for your Retroshare service is low. This can lead to slow communication and file sharing, impacting the performance of the service. 

### Troubleshoot the alert

1. Check the Retroshare service status

Make sure that the Retroshare service is running and has an active connection to the internet. You can verify this by checking the service logs or by accessing the Retroshare interface.

2. Inspect the network configuration

Verify that your Retroshare service can connect to the required ports for DHT (UDP) to function correctly. Also, ensure the ports are open in any firewall or security software.

3. Increase the number of bootstrap nodes

Retroshare requires a list of bootstrap nodes for the initial connection to the DHT network. If the current bootstrap nodes are not sufficient or unresponsive, try adding more bootstrap nodes to the list.

4. Update your Retroshare software

Older versions of the Retroshare service may not connect correctly and might have outdated DHT peers list. Ensure your Retroshare service is up-to-date and working with the latest version.

5. Check the Retroshare community

If you continue to experience issues with the DHT peer count, visit the Retroshare community forums or support channels to see if other users have encountered similar issues and whether any solutions are suggested.

### Useful resources

1. [Retroshare Official Website](https://retroshare.cc/)
2. [Retroshare GitHub Repository](https://github.com/RetroShare/RetroShare)
