### Understand the alert

This alert, `docker_container_unhealthy`, is triggered when the health status of a Docker container is marked as unhealthy. If you receive this alert, it means that one of your Docker containers is not functioning properly, which can affect the services or applications running inside the container.

### What does container health status mean?

The container health status is a Docker feature that allows you to define custom health checks to verify the proper functioning of your containers. If a container has a health check defined, Docker will execute it at regular intervals to monitor the container's health. If the health check fails a specific number of times in a row, Docker will mark the container as unhealthy, and this alert will be triggered.

### Troubleshoot the alert

1. Identify the affected container:

   Find the container name in the alert's info field: `${label:container_name} docker container health status is unhealthy`. Use this container name in the following steps.

2. Check the logs of the affected container:

   Use the `docker logs` command to view the logs of the unhealthy container. This may provide information on what caused the container to become unhealthy.

   ```
   docker logs <container_name>
   ```

3. Inspect the container's health check configuration:

   Use the `docker inspect` command to view the health check settings for the affected container. Look for any misconfigurations that could lead to the container being marked as unhealthy.

   ```
   docker inspect <container_name> --format='{{json .Config.Healthcheck}}'
   ```

4. Check the container's health status history:

   Use the `docker inspect` command again to review the health check history for the affected container.

   ```
   docker inspect <container_name> --format='{{json .State.Health}}'
   ```

5. Investigate and fix container issues:

   Based on the information gathered from the previous steps, investigate and fix any issues with the container's service, configuration, or resources. You might need to restart the container or reconfigure its health check settings.

   ```
   docker restart <container_name>
   ```

### Useful resources

1. [Docker's HEALTHCHECK instruction](https://stackoverflow.com/questions/38546755/how-to-use-dockers-healthcheck-instruction)
