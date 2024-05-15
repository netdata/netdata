# API Tokens

## Overview

Every single user can get access to the Netdata resource programmatically. It is done through the API Token which
can be also called as Bearer Token. This token is used for authentication and authorization, it can be issued
in the Netdata UI under the user Settings:

<img width="316" alt="image" src="https://github.com/netdata/netdata/assets/14999928/b0846076-afae-47ab-92df-c24967305ab9"/>

The API Tokens are not going to expire and can be limited to a few scopes:

* `scope:all`

  this token is given the same level of action as the user has, the use-case for it is Netdata terraform provider

* `scope:agent-ui`

  this token is mainly used by the local Netdata agent accessing the Cloud UI

* `scope:grafana-plugin`

  this token is used for the [Netdata Grafana plugin](https://github.com/netdata/netdata-grafana-datasource-plugin/blob/master/README.md)
  to access Netdata charts

Currently, the Netdata Cloud is not exposing stable API.

## Example usage

* get the cloud space list

```console
$ curl -H 'Accept: application/json' -H "Authorization: Bearer <token>" https://app.netdata.cloud/api/v2/spaces
```
