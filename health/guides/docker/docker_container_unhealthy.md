# docker_containers_unhealthy

**Containers | Docker**

_Docker is an open source containerization platform. It enables developers to package applications
into containers—standardized executable components combining application source code with the
operating system (OS) libraries and dependencies required to run that code in any environment_

Sometimes while our container is running, the application inside may have crashed. To foresee those
events, container runtimes (CR) and orchestrators perform health checks to endpoints inside the
functional units of the container. A container marked as unhealthy by the CR, is malfunctioning and
should be stopped. Those health checks are defined by the creator of the container with the
HEALTHCHECK
instructions. <sup>[1](https://docs.docker.com/engine/reference/builder/#healthcheck) </sup>

The Netdata Agent monitors the average number of unhealthy docker containers over the last 10
seconds. This alert indicates that some containers are not running due to failed health checks.

This alert is raised into warning when at least one container is unhealthy in your Docker engine.

<details>
<summary>References and sources</summary>

1. [HEALTHCHECK instruction in Docker docs](https://docs.docker.com/engine/reference/builder/#healthcheck)

</details>

### Troubleshooting section

<details>
<summary>Inspect and restart the UNHEALTY container</summary>

1. Check all the containers in the system.

    ```
    root@netdata # docker ps -a
    ```

2. Find the NAME of the container that is marked as UNHEALTHY.

3. Check the logs of this container to get some insights into what's going wrong

    ```
    root@netdata # docker logs <UNHEALTHY_CONTAINER>
    ```
   In many cases, your app's logs may not appear in docker log collector. A simple workaround is
   something like
   this, [redirect your apps's logs into stderr](https://github.com/nginxinc/docker-nginx/blob/master/Dockerfile-debian.template#L90)
   . Use this workaround purposefully. Another workaround is to redirect any log attempt to log
   directly into the `/proc/self/fd/2`.


4. Restart the container and see if this fixes the problem.

    ```
    root@netdata # docker logs <UNHEALTHY_CONTAINER>
    ```

5. If you receive this alert often, you may have to do further investigation on why this event occurs

</details>
