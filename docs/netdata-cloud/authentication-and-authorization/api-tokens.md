# API Tokens

Every user can get access to the Netdata resource programmatically. It is done through an API Token, also called "Bearer Token". This token is used for authentication and authorization, and it can be issued in the Netdata UI under the User Settings (at the profile picture on the bottom-left):

<img width="316" alt="image" src="https://github.com/netdata/netdata/assets/14999928/b0846076-afae-47ab-92df-c24967305ab9"/>

The API Tokens are not going to expire and can be limited to a few scopes:

| scope                  | description                                                                                                                                        |
|:-----------------------|:---------------------------------------------------------------------------------------------------------------------------------------------------|
| `scope:all`            | Token is given the same level of action as the user has, the use-case for it is Netdata terraform provider                                         |
| `scope:agent-ui`       | this token is mainly used by the local Agent for accessing the Cloud UI                                                                            |
| `scope:grafana-plugin` | Used for the [Netdata Grafana plugin](https://github.com/netdata/netdata-grafana-datasource-plugin/blob/master/README.md) to access Netdata charts |

> **Info**
>
> Currently, Netdata Cloud is not exposing the stable API.

## Example usage

**get the Netdata Cloud space list**

```console
curl -H 'Accept: application/json' -H "Authorization: Bearer <token>" https://app.netdata.cloud/api/v2/spaces
```
