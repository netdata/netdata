<!--
title: "Contributing to documentation"
description: "Want to contribute to Netdata's documentation? This guide will set you up with the tools to help others learn about health and performance monitoring."
custom_edit_url: https://github.com/netdata/netdata/edit/master/docs/contributing/contributing-documentation.md
-->

# Contributing to documentation

We welcome contributions to Netdata's already extensive documentation.

We store documentation related to the open-source Netdata Agent inside of the [`netdata/netdata`
repository](https://github.com/netdata/netdata) on GitHub. Documentation related to Netdata Cloud is stored in a private
repository and is not currently open to community contributions.

The Netdata team aggregates and publishes all documentation at [learn.netdata.cloud](https://learn.netdata.cloud/) using
[Docusaurus](https://v2.docusaurus.io/) in a private GitHub repository.

## Before you get started

Anyone interested in contributing to documentation should first read the [Netdata style
guide](/docs/contributing/style-guide.md) and the [Netdata Community Code of Conduct](https://learn.netdata.cloud/contribute/code-of-conduct).

Netdata's documentation uses Markdown syntax. If you're not familiar with Markdown, read the [Mastering
Markdown](https://guides.github.com/features/mastering-markdown/) guide from GitHub for the basics on creating
paragraphs, styled text, lists, tables, and more.

### Netdata's documentation structure

Netdata's documentation is separated into four sections.

- **Netdata**: Documents based on the actions users want to take, and solutions to their problems, such both the Netdata
  Agent and Netdata Cloud.
  - Stored in various subfolders of the [`/docs` folder](https://github.com/netdata/netdata/tree/master/docs) within the
    `netdata/netdata` repository: `/docs/collect`, `/docs/configure`, `/docs/export`, `/docs/get`, `/docs/monitor`,
    `/docs/overview`, `/docs/quickstart`, `/docs/store`, and `/docs/visualize`.
  - Published at [`https://learn.netdata.cloud/docs`](https://learn.netdata.cloud/docs).
- **Netdata Agent reference**: Reference documentation for the open-source Netdata Agent.
  - Stored in various `.md` files within the `netdata/netdata` repository alongside the code responsible for that
    feature. For example, the database engine's reference documentation is at `/database/engine/README.md`.
  - Published under the **Reference** section in the Netdata Learn sidebar.
- **Netdata Cloud reference**: Reference documentation for the closed-source Netdata Cloud web application.
  - Stored in a private GitHub repository and not editable by the community.
  - Published at [`https://learn.netdata.cloud/docs/cloud`](https://learn.netdata.cloud/docs/cloud).
- **Guides**: Solutions-based articles for users who want instructions on completing a specific complex task using the
  Netdata Agent and/or Netdata Cloud.
  - Stored in the [`/docs/guides` folder](https://github.com/netdata/netdata/tree/master/docs/guides) within the
    `netdata/netdata` repository. Organized into subfolders that roughly correlate with the core Netdata documentation.
  - Published at [`https://learn.netdata.cloud/guides`](https://learn.netdata.cloud/guides).

Generally speaking, if you want to contribute to the reference documentation for a specific Netdata Agent feature, find
the appropriate `.md` file co-located with that feature. If you want to contribute documentation that spans features or
products, or has no direct correlation with the existing directory structure, place it in the `/docs` folder within
`netdata/netdata`.

## How to contribute

The easiest way to contribute to Netdata's documentation is to edit a file directly on GitHub. This is perfect for small
fixes to a single document, such as fixing a typo or clarifying a confusing sentence.

Click on the **Edit this page** button on any published document on [Netdata Learn](https://learn.netdata.cloud). Each
page has two of these buttons: One beneath the table of contents, and another at the end of the document, which take you
to GitHub's code editor. Make your suggested changes, keeping [Netdata style guide](/docs/contributing/style-guide.md)
in mind, and use *Preview changes** button to ensure your Markdown syntax works as expected.

Under the **Commit changes**  header, write descriptive title for your requested change. Click the **Commit changes**
button to initiate your pull request (PR).

Jump down to our instructions on [PRs](#making-a-pull-request) for your next steps.

### Edit locally

Editing documentation locally is the preferred method for complex changes that span multiple documents or change the
documentation's style or structure.

Create a fork of the Netdata Agent repository by visit the [Netdata repository](https://github.com/netdata/netdata) and
clicking on the **Fork** button. 

![Screenshot of forking the Netdata
repository](https://user-images.githubusercontent.com/1153921/59873572-25f5a380-9351-11e9-92a4-a681fe4a2ed9.png)

GitHub will ask you where you want to clone the repository. When finished, you end up at the index of your forked
Netdata Agent repository. Clone your fork to your local machine:

```bash
git clone https://github.com/YOUR-GITHUB-USERNAME/netdata.git
```

Create a new branch using `git checkout -b BRANCH-NAME`. Use your favorite text editor to make your changes, keeping the
[Netdata style guide](/docs/contributing/style-guide.md) in mind. Add, commit, and push changes to your fork. When
you're finished, visit the [Netdata Agent Pull requests](https://github.com/netdata/netdata/pulls) to create a new pull
request based on the changes you made in the new branch of your fork.

## Making a pull request

Pull requests (PRs) should be concise and informative. See our [PR guidelines](https://learn.netdata.cloud/contribute/handbook#pr-guidelines) for
specifics.

- The title must follow the [imperative mood](https://en.wikipedia.org/wiki/Imperative_mood) and be no more than ~50
  characters.
- The description should explain what was changed and why. Verify that you tested any code or processes that you are
  trying to change.

The Netdata team will review your PR and assesses it for correctness, conciseness, and overall quality. We may point to
specific sections and ask for additional information or other fixes.

After merging your PR, the Netdata team rebuilds the [documentation site](https://learn.netdata.cloud) to publish the
changed documentation.


