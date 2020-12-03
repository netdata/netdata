<!--
title: "Contributing to documentation"
description: "Want to contribute to Netdata's documentation? This guide will set you up with the tools to help others learn about health and performance monitoring."
custom_edit_url: https://github.com/netdata/netdata/edit/master/docs/contributing/contributing-documentation.md
-->

# Contributing to documentation

We welcome contributions to Netdata's already extensive documentation. We store documentation related to our open source
monitoring Agent inside of the [`netdata/netdata` repository](https://github.com/netdata/netdata) on GitHub.

Documentation related to Netdata Cloud is stored in a private repository and is not currently open to community
contributions.

All our documentation is aggregated and published at [learn.netdata.cloud](https://learn.netdata.cloud/).

Like all contributing to all other aspects of Netdata, we ask that anyone who wants to help with documentation read and
abide by the [Contributor Convenant Code of Conduct](/CODE_OF_CONDUCT.md) and follow the
instructions outlined in our [Contributing document](/CONTRIBUTING.md).

We also ask you to read our [documentation style guide](/docs/contributing/style-guide.md), which, while not complete,
will give you some guidance on how we write and organize our documentation.

All our documentation uses the Markdown syntax. If you're not familiar with how it works, please read the [Markdown
introduction post](https://daringfireball.net/projects/markdown/) by its creator, followed by [Mastering
Markdown](https://guides.github.com/features/mastering-markdown/) guide from GitHub.

Our [documentation site](https://learn.netdata.cloud/) is built with [Docusaurus](https://v2.docusaurus.io/). If you
want to contribute to the generated documentation site, and have experience in React and/or Docusaurus itself, send us
an email: [info@netdata.cloud](mailto:info@netdata.cloud).

## How contributing to the documentation works

There are two ways to contribute to Netdata's documentation: 

1.  Edit documentation [directly in GitHub](#edit-documentation-directly-on-github).
2.  Download the repository and [edit documentation locally](#edit-documentation-locally).

Editing in GitHub is a more straightforward process and is perfect for quick edits to a single document, such as fixing
a typo or clarifying a confusing sentence.

Editing locally is more complicated but allows you to organize complex projects. By building documentation locally, you
can preview your work using a local web server before you submit your PR.

In both cases, you'll finish by submitting a pull request (PR). Once you submit your PR, GitHub will initiate a number
of jobs, including a Netlify preview. You can use this preview to view the documentation site with your changes applied,
which might help you catch any lingering issues.

To continue, follow one of the paths below:

-   [Edit documentation directly in GitHub](#edit-documentation-directly-on-github)
-   [Edit documentation locally](#edit-documentation-locally)

## Edit documentation directly on GitHub

Start editing documentation on GitHub by clicking the small pencil icon on any page on Netdata's [documentation
site](https://learn.netdata.cloud/). You can find them at the top of every page.

Clicking on this icon will take you to the associated page in the `netdata/netdata` repository. Then click the small
pencil icon on any documentation file (those ending in the `.md` Markdown extension) in the `netdata/netdata`
repository.

![A screenshot of editing a Markdown file directly in the Netdata
repository](https://user-images.githubusercontent.com/1153921/59637188-10426d00-910a-11e9-99f2-ec564d6fb7d5.png)

If you know where a file resides in the Netdata repository already, you can skip the step of beginning on the
documentation site and go directly to GitHub.

Once you've clicked the pencil icon on GitHub, you'll see a full Markdown version of the file. Make changes as you see
fit. You can use the **Preview changes** button to ensure your Markdown syntax is working correctly.

Under the **Propose file change** header, write in a descriptive title for your requested change. Beneath that, add a
concise description of what you've changed and why you think it's essential. Then, click the **Propose file change**
button.

After you've hit that button, jump down to our instructions on [pull requests and
cleanup](#pull-requests-and-final-steps) for your next steps.

## Edit documentation locally

Editing documentation locally is the preferred method for complex changes, PRs that span across multiple documents, or
those that change the documentation's style or underlying functionality.

Here is the workflow for editing documentation locally. First, create a fork of the Netdata repository, if you don't
have one already. Visit the [Netdata repository](https://github.com/netdata/netdata) and click on the `Fork` button in
the upper-right corner of the window.

![Screenshot of forking the Netdata
repository](https://user-images.githubusercontent.com/1153921/59873572-25f5a380-9351-11e9-92a4-a681fe4a2ed9.png)

GitHub will ask you where you want to clone the repository, and once finished, you end up at the index of your forked
Netdata repository. Clone your fork to your local machine:

```bash
git clone https://github.com/YOUR-GITHUB-USERNAME/netdata.git
```

You can now jump into the directory and explore Netdata's structure for yourself.

### Understanding the structure of Netdata's documentation

All of Netdata's documentation is stored within the `netdata/netdata` repository, as close as possible to the code it
corresponds to. Many sub-folders contain a `README.md` file, which is then used to populate the documentation about that
feature/component of Netdata.

For example, the installation documentation at `packaging/installer/README.md` becomes
`https://learn.netdata.cloud/docs/agent/packaging/installer/`. By co-locating it with quick-start installation code, we
ensure documentation is always tightly-knit with the functions it describes.

You might find other `.md` files within these directories. The `packaging/installer/` folder also contains `UPDATE.md`
and `UNINSTALL.md`, which become `https://learn.netdata.cloud/docs/agent/packaging/installer/update/` and
`https://learn.netdata.cloud/docs/agent/packaging/installer/uninstall/`, respectively.

If the documentation you're working on has a direct correlation to some component of Netdata, place it into the correct
folder and either name it `README.md` for generic documentation, or with another name for very specific instructions.

#### The `docs` folder

At the root of the Netdata repository is a `docs/` folder. Inside this folder, we place documentation that does not have
a direct relationship to a specific component of Netdata.

If the documentation you're working on doesn't have a direct relationship to a component of Netdata, you can place it in
this `docs/` folder.

These documents will end up at the "root" of the Agent documentation at `https://learn.netdata.cloud/docs/agent/`. For
example, the file at `docs/getting-started.md` becomes `https://learn.netdata.cloud/docs/agent/getting-started/`.

### Make your edits

Now that you're set up and understand where to find or create your `.md` file, you can now begin to make your edits.
Just use your favorite editor and keep in mind our [style guide](style-guide.md) as you work.

Be sure to periodically add/commit your edits so that you don't lose your work! We use version control software for a
reason.

## Pull requests and final steps

When you finish with your changes, add and commit them to your fork of the Netdata repository. Head over to GitHub to
create your pull request (PR).

Once we receive your pull request (PR), the Netdata team reads through it and assesses it for correctness, conciseness,
and overall quality. We may point to specific sections and ask for additional information or other fixes.

After merging your PR, we then rebuild the [documentation site](https://learn.netdata.cloud), which is built with
[Docusaurus](https://v2.docusaurus.io/). You can then also delete your branch 

## What's next

-   Read up on the Netdata documentation [style guide](style-guide.md).

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fdocs%2Fcontributing%2Fcontributing-documentation&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
