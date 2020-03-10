<!--
---
title: "Agent claiming"
custom_edit_url: https://github.com/netdata/netdata/edit/master/claim/README.md
---
-->

# Agent claiming

Agent claiming is part of the onboarding process when creating a workspace in Netdata Cloud. Each workspace gets its own
common invitation mechanism, which begins with the administrators of the workspace creating a **claiming-token**. They,
or other users is their organization, can then use the claiming-token to add an agent to their workspace.

To claim a Netdata agent, you first send a claiming request to Netdata Cloud (from the agent node). Once the
Netdata Cloud validates the claiming request of the agent (based on the claiming token), and returns a successful
result, the node is considered claimed.

## Claiming script

The user can claim an agent by directly calling the `netdata-claim.sh` script **as the netdata user** and passing the
following arguments:

```sh
-token=TOKEN
    where TOKEN is the workspace claiming-token.
-rooms=ROOM1,ROOM2,...
    where ROOMX is the workspace war-room to join. This list is optional.
-url=URL_BASE
    where URL_BASE is the Netdata Cloud endpoint base URL. By default, this is https://netdata.cloud.
-id=AGENT_ID
    where AGENT_ID is the unique identifier of the agent. This is the agent's MACHINE_GUID by default.
-hostname=HOSTNAME
    where HOSTNAME is the result of the hostname command by default.
```

For example, the following command claims an agent and adds it to rooms `room1` and `room2`:

```sh
netdata-claim.sh -token=MYTOKEN1234567 -rooms=room1,room2
```

You should then update the `netdata` service about the result with `netdatacli`:

```sh
netdatacli reload-claiming-state
```

This reloads the agent claiming state from disk.

## Netdata agent command line

The user can trigger agent claiming by calling the `netdata` service binary with the additional command line parameters:

```sh
-W "claim -token=TOKEN -rooms=ROOM1,ROOM2"
```

For example:

```sh
/usr/sbin/netdata -D -W "claim -token=MYTOKEN1234567 -rooms=room1,room2"
```

If need be, the user can override the agent's defaults by providing additional arguments like those described
[here](#claiming-script).

## Claiming directory

Netdata stores the agent claiming-related state in the user configuration directory under `claim.d`, e.g. in
`/etc/netdata/claim.d`. The user can put files in this directory to provide defaults to the `-token` and `-rooms`
arguments. These files should be owned **by the `netdata` user**.

The `claim.d/token` file should contain the claiming-token and the `claim.d/rooms` file should contain the list of 
war-rooms.

The user can also put the Cloud endpoint's full certificate chain in `claim.d/cloud_fullchain.pem` so that the agent
can trust the endpoint if necessary.

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fclaim%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
