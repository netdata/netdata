# Netdata APIs

Netdata provides multiple APIs for programmatic access to your monitoring infrastructure.

:::note

API documentation is a work in progress. More endpoints will be added soon.

:::

## Agent API

The Netdata Agent REST API provides access to metrics, alerts, and configuration on individual nodes. The complete documentation is available in OpenAPI format.

Explore the Agent API using:

- **[Swagger UI](https://learn.netdata.cloud/api)** - Interactive API explorer
- **[Swagger Editor](https://editor.swagger.io/?url=https://raw.githubusercontent.com/netdata/netdata/master/src/web/api/netdata-swagger.yaml)** - Edit and test API calls
- **[OpenAPI Specification](https://raw.githubusercontent.com/netdata/netdata/master/src/web/api/netdata-swagger.yaml)** - Raw OpenAPI YAML

## Cloud API

The Netdata Cloud REST API provides programmatic access to Cloud resources, spaces, war rooms, and nodes across your infrastructure.

Explore the Cloud API using the live API documentation:

- **[Cloud API Documentation](https://app.netdata.cloud/api/docs/)** - Interactive API explorer

:::tip

The Cloud API documentation is always up-to-date and allows you to discover endpoints, view payloads, and test requests directly in your browser.

:::

### Generate an API Token

To use the Cloud API, generate an API token from your Netdata Cloud account.

#### Step 1: Access Account Settings

From the profile menu, click on **"Settings"** to access your account settings page.

![settings](https://raw.githubusercontent.com/netdata/docs-images/refs/heads/master/netdata-cloud/account-management/delete-account/profile-menu-settings.png)

#### Step 2: Create a New Token

In the API Tokens section, click the **+** button to create a new token.

![create token](https://raw.githubusercontent.com/netdata/docs-images/3432ca2fff3ef3ea44948d781713d56a3143fc66/%2B2.png)

#### Step 3: Select Token Scope

Choose `scope:all` to grant the token access to all Cloud API endpoints. Other scopes provide access to specific subsets of endpoints depending on your use case.

![token scope](https://raw.githubusercontent.com/netdata/docs-images/3432ca2fff3ef3ea44948d781713d56a3143fc66/MyAPI.png)

#### Step 4: Save Your Token

The token generates and displays once for security reasons. Save it securely - you'll need to generate a new token if you lose it.

![generated token](https://raw.githubusercontent.com/netdata/docs-images/3432ca2fff3ef3ea44948d781713d56a3143fc66/Token%20Generated.png)

:::warning

Save your token immediately. Netdata Cloud shows it only once for security reasons. If you lose it, generate a new token.

:::

#### Step 5: Authenticate API Requests

Include the token in your API request headers:

```
Authorization: Bearer {{your_token}}
```

Replace `{{your_token}}` with your actual API token.

**Example request:**

```bash
curl -X GET "https://app.netdata.cloud/api/v2/spaces" \
  -H "Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
```

![authorization header](https://raw.githubusercontent.com/netdata/docs-images/3432ca2fff3ef3ea44948d781713d56a3143fc66/Available%20Authorizations2.png)
