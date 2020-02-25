# `@actions/github`

> A hydrated Octokit client.

## Usage

Returns an authenticated Octokit client that follows the machine [proxy settings](https://help.github.com/en/actions/automating-your-workflow-with-github-actions/about-self-hosted-runners#using-a-proxy-server-with-self-hosted-runners). See https://octokit.github.io/rest.js for the API.

```js
const github = require('@actions/github');
const core = require('@actions/core');

async function run() {
    // This should be a token with access to your repository scoped in as a secret.
    // The YML workflow will need to set myToken with the GitHub Secret Token
    // myToken: ${{ secrets.GITHUB_TOKEN }}
    // https://help.github.com/en/actions/automating-your-workflow-with-github-actions/authenticating-with-the-github_token#about-the-github_token-secret
    const myToken = core.getInput('myToken');

    const octokit = new github.GitHub(myToken);

    const { data: pullRequest } = await octokit.pulls.get({
        owner: 'octokit',
        repo: 'rest.js',
        pull_number: 123,
        mediaType: {
          format: 'diff'
        }
    });

    console.log(pullRequest);
}

run();
```

You can pass client options, as specified by [Octokit](https://octokit.github.io/rest.js/), as a second argument to the `GitHub` constructor.

You can also make GraphQL requests. See https://github.com/octokit/graphql.js for the API.

```js
const result = await octokit.graphql(query, variables);
```

Finally, you can get the context of the current action:

```js
const github = require('@actions/github');

const context = github.context;

const newIssue = await octokit.issues.create({
  ...context.repo,
  title: 'New issue!',
  body: 'Hello Universe!'
});
```

## Webhook payload typescript definitions

The npm module `@octokit/webhooks` provides type definitions for the response payloads. You can cast the payload to these types for better type information.

First, install the npm module `npm install @octokit/webhooks`

Then, assert the type based on the eventName
```ts
import * as core from '@actions/core'
import * as github from '@actions/github'
import * as Webhooks from '@octokit/webhooks'
if (github.context.eventName === 'push') {
  const pushPayload = github.context.payload as Webhooks.WebhookPayloadPush
  core.info(`The head commit is: ${pushPayload.head}`)
}
```
