# Agent Claiming

The claiming of nodes is a node onboarding process in a workspace, that is achieved by using a common invitation
mechanism per workspace. The invitation mechanism, includes creation of a claiming-token by the admins of the workspace.
An agent node can use the invitation token to join the workspace.

Claiming of a netdata node can be done by sending a claiming request to netdata cloud (from the agent node). Once the
netdata cloud validates the claiming request of the agent (based on the claiming token), and returns a successful
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
    where URL_BASE is the netdata cloud endpoint base URL. This is https://netdata.cloud by default.
-id=AGENT_ID
    where AGENT_ID is the unique identifier of the agent. This is the agent's MACHINE_GUID by default.
hostname=HOSTNAME
    where HOSTNAME is the result of of the hostname command by default.
```
One claiming command example is:
```sh
netdata-claim.sh -token=MYTOKEN1234567 -rooms=room1,room2
```

The `netdata` service is updated about the result by executing:
```sh
netdatacli reload-claiming-state
```
which reloads agent claiming state from disk.

# netdata agent command line

The user can trigger agent claiming by calling the `netdata` service binary with the additional command line parameters:
```sh
-W "claim -token=TOKEN -rooms=ROOM1,ROOM2"
```
For example:
```sh
/usr/sbin/netdata -D -W "claim -token=MYTOKEN1234567 -rooms=room1,room2"
```

If need be, the user can override the agent's defaults by providing additional arguments like those described
[here](#Claiming-script).

## Claiming directory

The agent claiming-related state is stored in the user configuration directory under `claim.d`, e.g. in
`/etc/netdata/claim.d`. The user can put files in this directory to provide defaults to the `-token` and `-rooms`
arguments. These files should be owned **by the netdata user**.

The `claim.d/token` file shall contain the claiming-token and the `claim.d/rooms` file shall contain the list of 
war-rooms. 

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fclaim%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
