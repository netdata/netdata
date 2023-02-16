# Docs Development Guidelines

Welcome to our docs developer guidelines!

We store documentation related to Netdata inside of
the [`netdata/netdata` repository](https://github.com/netdata/netdata) on GitHub.

The Netdata team aggregates and publishes all documentation at [learn.netdata.cloud](/) using
[Docusaurus](https://v2.docusaurus.io/) over at the [`netdata/learn` repository](https://github.com/netdata/learn).

## Before you get started

Anyone interested in contributing to documentation should first read the [Netdata style guide](#styling-guide) further
down below and the [Netdata Community Code of Conduct](https://github.com/netdata/.github/blob/main/CODE_OF_CONDUCT.md).

Netdata's documentation uses Markdown syntax. If you're not familiar with Markdown, read
the [Mastering Markdown](https://guides.github.com/features/mastering-markdown/) guide from GitHub for the basics on
creating paragraphs, styled text, lists, tables, and more, and read further down about some special
occasions [while writing in MDX](#mdx-and-markdown).

### Netdata's Documentation structure

Netdata's documentation is separated into 5 categories.

- **Getting Started**: This section’s purpose is to present “What is Netdata” and for whom is it for while also
  presenting all the ways Netdata can be deployed. That includes Netdata’s platform support, Standalone deployment,
  Parent-child deployments, deploying on Kubernetes and also deploying on IoT nodes.
    - Stored in **WIP**
    - Published in **WIP**
- **Concepts**: This section’s purpose is to take a pitch on all the aspects of Netdata. We present the functionality of
  each component/idea and support it with examples but we don’t go deep into technical details.
    - Stored in the `/docs/concepts` directory in the `netdata/netdata` repository.
    - Published in **WIP**
- **Tasks**: This section's purpose is to break down any operation into a series of fundamental tasks for the Netdata
  solution.
    - Stored in the `/docs/tasks` directory in the `netdata/netdata` repository.
    - Published in **WIP**
- **References**: This section’s purpose is to explain thoroughly every part of Netdata. That covers settings,
  configurations and so on.
    - Stored near the component they refer to.
    - Published in **WIP**
- **Collectors References**: This section’s purpose is to explain thoroughly every collector that Netdata supports and
  it's configuration options.
    - Stored in stored near the collector they refer to.
    - Published in **WIP**

## How to contribute

The easiest way to contribute to Netdata's documentation is to edit a file directly on GitHub. This is perfect for small
fixes to a single document, such as fixing a typo or clarifying a confusing sentence.

Click on the **Edit this page** button on any published document on [Netdata Learn](https://learn.netdata.cloud). Each
page has two of these buttons: One beneath the table of contents, and another at the end of the document, which take you
to GitHub's code editor. Make your suggested changes, keeping the [Netdata style guide](#styling-guide)
in mind, and use the ***Preview changes*** button to ensure your Markdown syntax works as expected.

Under the **Commit changes**  header, write descriptive title for your requested change. Click the **Commit changes**
button to initiate your pull request (PR).

Jump down to our instructions on [PRs](#making-a-pull-request) for your next steps.

### Edit locally

Editing documentation locally is the preferred method for complex changes that span multiple documents or change the
documentation's style or structure.

Create a fork of the Netdata Agent repository by visit the [Netdata repository](https://github.com/netdata/netdata) and
clicking on the **Fork** button.

GitHub will ask you where you want to clone the repository. When finished, you end up at the index of your forked
Netdata Agent repository. Clone your fork to your local machine:

```bash
git clone https://github.com/YOUR-GITHUB-USERNAME/netdata.git
```

Create a new branch using `git checkout -b BRANCH-NAME`. Use your favorite text editor to make your changes, keeping
the [Netdata style guide](https://github.com/netdata/netdata/blob/master/docs/contributing/style-guide.md) in mind. Add, commit, and push changes to your fork. When you're
finished, visit the [Netdata Agent Pull requests](https://github.com/netdata/netdata/pulls) to create a new pull request
based on the changes you made in the new branch of your fork.

### Making a pull request

Pull requests (PRs) should be concise and informative. See our [PR guidelines](/contribute/handbook#pr-guidelines) for
specifics.

- The title must follow the [imperative mood](https://en.wikipedia.org/wiki/Imperative_mood) and be no more than ~50
  characters.
- The description should explain what was changed and why. Verify that you tested any code or processes that you are
  trying to change.

The Netdata team will review your PR and assesses it for correctness, conciseness, and overall quality. We may point to
specific sections and ask for additional information or other fixes.

After merging your PR, the Netdata team rebuilds the [documentation site](https://learn.netdata.cloud) to publish the
changed documentation.

## Writing Docs

We have three main types of Docs: **References**, **Concepts** and **Tasks**.

### Metadata Tags

All of the Docs however have what we call "metadata" tags. these help to organize the document upon publishing.

So let's go through the different necessary metadata tags to get a document properly published on Learn:

- Docusaurus Specific:\
  These metadata tags are parsed automatically by Docusaurus and are rendered in the published document. **Note**:
  Netdata only uses the Docusaurus metadata tags releveant for our documentation infrastructure.
    - `title: "The title of the document"` : Here we specify the title of our document, which is going to be converted
      to the heading of the published page.
    - `description: "The description of the file"`: Here we give a description of what this file is about.
    - `custom_edit_url: https://github.com/netdata/netdata/edit/master/collectors/COLLECTORS.md`: Here is an example of
      the link that the user will be redirected to if he clicks the "Edit this page button", as you see it leads
      directly to the edit page of the source file.
- Netdata Learn specific:
    - `learn_status: "..."`
        - The options for this tag are:
            - `"published"`
            - `"unpublished"`
    - `learn_topic_type: "..."`
        - The options for this tag are:
            - `"Getting Started"`
            - `"Concepts"`
            - `"Tasks"`
            - `"References"`
            - `"Collectors References"`
        - This is the Topic that the file belongs to, and this is going to resemble the start directory of the file's
          path on Learn for example if we write `"Concepts"` in the field, then the file is going to be placed
          under `/Concepts/....` inside Learn.
    - `learn_rel_path: "/example/"`
        - This tag represents the rest of the path, without the filename in the end, so in this case if the file is a
          Concept, it would go under `Concepts/example/filename.md`. If you want to place the file under the "root"
          topic folder, input `"/"`.
    - ⚠️ In case any of these "Learn" tags are missing or falsely inputted the file will remain unpublished. This is by
      design to prevent non-properly tagged files from getting published.

While Docusaurus can make use of more metadata tags than the above, these are the minimum we require to publish the file
on Learn.

### Doc Templates

These are the templates we use for our Documentation files:

<details>
<summary>Reference Docs</summary>

The template that is used for Reference files is:

```
  <!--
  title: "Apache monitoring with Netdata"
  description: "Monitor the health and performance of Apache web servers with zero configuration, per-second metric granularity, and interactive visualizations."
  custom_edit_url: https://github.com/netdata/go.d.plugin/edit/master/modules/apache/README.md
  learn_topic_type: "Collector References"
  learn_rel_path: "/sample_category"
  learn_status: "published"
  -->
```

## Configuration files

### Data collection

```
go.d/apache.conf
```

To make changes, see `the ./edit-config task <link>`

### Alerts

none

## Requirements to run this module

- none

## Requirement on the monitored application

- `Apache` with enabled [`mod_status`](https://httpd.apache.org/docs/2.4/mod/mod_status.html)

## Auto detection

### Single node installation

. . . we autodetect localhost:port and what configurations are defaults

### Kubernetes installations

. . . Service discovery, click here

## Metrics

Columns: Context | description (of the context) | units (of the context) | dimensions | alerts

- Requests in `requests/s`
- Connections in `connections`
- Async Connections in `connections`
- Scoreboard in `connections`
- Bandwidth in `kilobits/s`
- Workers in `workers`
- Lifetime Average Number Of Requests Per Second in `requests/s`
- Lifetime Average Number Of Bytes Served Per Second in `KiB/s`
- Lifetime Average Response Size in `KiB`

### Labels

just like <https://github.com/netdata/go.d.plugin/tree/master/modules/k8s_state#labels>

## Alerts

collapsible content for every alert, just like the alert guides

## Configuration options

Table with all the configuration options available.

Columns: name | description | default

## Configuration example

Needs only `url` to server's `server-status?auto`. Here is an example for 2 servers:

```yaml
jobs:
  - name: local
      url: http://127.0.0.1/server-status?auto
  - name: remote
      url: http://203.0.113.10/server-status?auto
```

For all available options please see
module [configuration file](https://github.com/netdata/go.d.plugin/blob/master/config/go.d/apache.conf).

## Troubleshoot

backlink to the task to run this module in debug mode

</details>

<details>
<summary>Task Docs</summary>

The template that is used for Task files is:

```
  <!--
  title: "Task title"
  description: "Task description"
  custom_edit_url: https://github.com/netdata/netdata/edit/master/docs/Tasks/sampletask.md
  learn_topic_type: "Tasks"
  learn_rel_path: "/sample_category"
  learn_status: "published"
  -->
```

## Description

A small description of the Task.

## Prerequisites

Describe all the information that the user needs to know before proceeding with the task.

## Context

Describe the background information of the Task, the purpose of the Task, and what will the user achieve by completing
it.

## Steps

A task consists of steps, here provide the actions needed from the user, so he can complete the task correctly.

## Result

Describe the expected output/ outcome of the result.

## Example

Provide any examples needed for the Task

</details>

<details>
<summary>Concept Docs</summary>

The template of the Concept files is:

```
  <!--
  title: "Concept title"
  description: "Concept description"
  custom_edit_url: https://github.com/netdata/netdata/edit/master/docs/Concepts/sampleconcept.md
  learn_topic_type: "Concepts"
  learn_rel_path: "/sample_category"
  learn_status: "published"
  -->
```

## Description

In our concepts we have a more loose structure, the goal is to communicate the "concept" to the user, starting with
simple language that even a new user can understand, and building from there.

</details>

## Styling Guide

The *Netdata style guide* establishes editorial guidelines for any writing produced by the Netdata team or the Netdata
community, including documentation, articles, in-product UX copy, and more. Both internal Netdata teams and external
contributors to any of Netdata's open-source projects should reference and adhere to this style guide as much as
possible.

Netdata's writing should **empower** and **educate**. You want to help people understand Netdata's value, encourage them
to learn more, and ultimately use Netdata's products to democratize monitoring in their organizations. To achieve these
goals, your writing should be:

- **Clear**. Use simple words and sentences. Use strong, direct, and active language that encourages readers to action.
- **Concise**. Provide solutions and answers as quickly as possible. Give users the information they need right now,
  along with opportunities to learn more.
- **Universal**. Think of yourself as a guide giving a tour of Netdata's products, features, and capabilities to a
  diverse group of users. Write to reach the widest possible audience.

You can achieve these goals by reading and adhering to the principles outlined below.

## Voice and tone

One way we write empowering, educational content is by using a consistent voice and an appropriate tone.

*Voice* is like your personality, which doesn't really change day to day.

*Tone* is how you express your personality. Your expression changes based on your attitude or mood, or based on who
you're around. In writing, your reflect tone in your word choice, punctuation, sentence structure, or even the use of
emoji.

The same idea about voice and tone applies to organizations, too. Our voice shouldn't change much between two pieces of
content, no matter who wrote each, but the tone might be quite different based on who we think is reading.

For example, a [blog post](https://www.netdata.cloud/blog/) and a [press release](https://www.netdata.cloud/news/)
should have a similar voice, despite most often being written by different people. However, blog posts are relaxed and
witty, while press releases are focused and academic. You won't see any emoji in a press release.

### Voice

Netdata's voice is authentic, passionate, playful, and respectful.

- **Authentic** writing is honest and fact-driven. Focus on Netdata's strength while accurately communicating what
  Netdata can and cannot do, and emphasize technical accuracy over hard sells and marketing jargon.
- **Passionate** writing is strong and direct. Be a champion for the product or feature you're writing about, and let
  your unique personality and writing style shine.
- **Playful** writing is friendly, thoughtful, and engaging. Don't take yourself too seriously, as long as it's not at
  the expense of Netdata or any of its users.
- **Respectful** writing treats people the way you want to be treated. Prioritize giving solutions and answers as
  quickly as possible.

### Tone

Netdata's tone is fun and playful, but clarity and conciseness comes first. We also tend to be informal, and aren't
afraid of a playful joke or two.

While we have general standards for voice and tone, we do want every individual's unique writing style to reflect in
published content.

## Universal communication

Netdata is a global company in every sense, with employees, contributors, and users from around the world. We strive to
communicate in a way that is clear and easily understood by everyone.

Here are some guidelines, pointers, and questions to be aware of as you write to ensure your writing is universal. Some
of these are expanded into individual sections in
the [language, grammar, and mechanics](#language-grammar-and-mechanics) section below.

- Would this language make sense to someone who doesn't work here?
- Could someone quickly scan this document and understand the material?
- Create an information hierarchy with key information presented first and clearly called out to improve scannability.
- Avoid directional language like "sidebar on the right of the page" or "header at the top of the page" since
  presentation elements may adapt for devices.
- Use descriptive links rather than "click here" or "learn more".
- Include alt text for images and image links.
- Ensure any information contained within a graphic element is also available as plain text.
- Avoid idioms that may not be familiar to the user or that may not make sense when translated.
- Avoid local, cultural, or historical references that may be unfamiliar to users.
- Prioritize active, direct language.
- Avoid referring to someone's age unless it is directly relevant; likewise, avoid referring to people with age-related
  descriptors like "young" or "elderly."
- Avoid disability-related idioms like "lame" or "falling on deaf ears." Don't refer to a person's disability unless
  it’s directly relevant to what you're writing.
- Don't call groups of people "guys." Don't call women "girls."
- Avoid gendered terms in favor of neutral alternatives, like "server" instead of "waitress" and "businessperson"
  instead of "businessman."
- When writing about a person, use their communicated pronouns. When in doubt, just ask or use their name. It's OK to
  use "they" as a singular pronoun.

> Some of these guidelines were adapted from MailChimp under the Creative Commons license.

## Language, grammar, and mechanics

To ensure Netdata's writing is clear, concise, and universal, we have established standards for language, grammar, and
certain writing mechanics. However, if you're writing about Netdata for an external publication, such as a guest blog
post, follow that publication's style guide or standards, while keeping
the [preferred spelling of Netdata terms](#netdata-specific-terms) in mind.

### Active voice

Active voice is more concise and easier to understand compared to passive voice. When using active voice, the subject of
the sentence is action. In passive voice, the subject is acted upon. A famous example of passive voice is the phrase
"mistakes were made."

|                 |                                                                                           |
|-----------------|-------------------------------------------------------------------------------------------|
| Not recommended | When an alarm is triggered by a metric, a notification is sent by Netdata.                |
| **Recommended** | When a metric triggers an alarm, Netdata sends a notification to your preferred endpoint. |

### Second person

Use the second person ("you") to give instructions or "talk" directly to users.

In these situations, avoid "we," "I," "let's," and "us," particularly in documentation. The "you" pronoun can also be
implied, depending on your sentence structure.

One valid exception is when a member of the Netdata team or community wants to write about said team or community.

|                                |                                                              |
|--------------------------------|--------------------------------------------------------------|
| Not recommended                | To install Netdata, we should try the one-line installer...  |
| **Recommended**                | To install Netdata, you should try the one-line installer... |
| **Recommended**, implied "you" | To install Netdata, try the one-line installer...            |

### "Easy" or "simple"

Using words that imply the complexity of a task or feature goes against our policy
of [universal communication](#universal-communication). If you claim that a task is easy and the reader struggles to
complete it, you may inadvertently discourage them.

However, if you give users two options and want to relay that one option is genuinely less complex than another, be
specific about how and why.

For example, don't write, "Netdata's one-line installer is the easiest way to install Netdata." Instead, you might want
to say, "Netdata's one-line installer requires fewer steps than manually installing from source."

### Slang, metaphors, and jargon

A particular word, phrase, or metaphor you're familiar with might not translate well to the other cultures featured
among Netdata's global community. We recommended you avoid slang or colloquialisms in your writing.

In addition, don't use abbreviations that have not yet been defined in the content. See our section on
[abbreviations](#abbreviations-acronyms-and-initialisms) for additional guidance.

If you must use industry jargon, such as "mean time to resolution," define the term as clearly and concisely as you can.

> Netdata helps you reduce your organization's mean time to resolution (MTTR), which is the average time the responsible
> team requires to repair a system and resolve an ongoing incident.

### Spelling

While the Netdata team is mostly *not* American, we still aspire to use American spelling whenever possible, as it is
the standard for the monitoring industry.

See the [word list](#word-list) for spellings of specific words.

### Capitalization

Follow the general [English standards](https://owl.purdue.edu/owl/general_writing/mechanics/help_with_capitals.html) for
capitalization. In summary:

- Capitalize the first word of every new sentence.
- Don't use uppercase for emphasis. (Netdata is the BEST!)
- Capitalize the names of brands, software, products, and companies according to their official guidelines. (Netdata,
  Docker, Apache, NGINX)
- Avoid camel case (NetData) or all caps (NETDATA).

Whenever you refer to the company Netdata, Inc., or the open-source monitoring agent the company develops, capitalize
**Netdata**.

However, if you are referring to a process, user, or group on a Linux system, use lowercase and fence the word in an
inline code block: `` `netdata` ``.

|                 |                                                                                                |
|-----------------|------------------------------------------------------------------------------------------------|
| Not recommended | The netdata agent, which spawns the netdata process, is actively maintained by netdata, inc.   |
| **Recommended** | The Netdata Agent, which spawns the `netdata` process, is actively maintained by Netdata, Inc. |

#### Capitalization of document titles and page headings

Document titles and page headings should use sentence case. That means you should only capitalize the first word.

If you need to use the name of a brand, software, product, and company, capitalize it according to their official
guidelines.

Also, don't put a period (`.`) or colon (`:`) at the end of a title or header.

|                 |                                                                                                     |
|-----------------|-----------------------------------------------------------------------------------------------------|
| Not recommended | Getting Started Guide <br />Service Discovery and Auto-Detection: <br />Install netdata with docker |
| **Recommended** | Getting started guide <br />Service discovery and auto-detection <br />Install Netdata with Docker  |

### Abbreviations (acronyms and initialisms)

Use abbreviations (including [acronyms and initialisms](https://www.dictionary.com/e/acronym-vs-abbreviation/)) in
documentation when one exists, when it's widely accepted within the monitoring/sysadmin community, and when it improves
the readability of a document.

When introducing an abbreviation to a document for the first time, give the reader both the spelled-out version and the
shortened version at the same time. For example:

> Use Netdata to monitor Extended Berkeley Packet Filter (eBPF) metrics in real-time.
> After you define an abbreviation, don't switch back and forth. Use only the abbreviation for the rest of the document.

You can also use abbreviations in a document's title to keep the title short and relevant. If you do this, you should
still introduce the spelled-out name alongside the abbreviation as soon as possible.

### Clause order

When instructing users to take action, give them the context first. By placing the context in an initial clause at the
beginning of the sentence, users can immediately know if they want to read more, follow a link, or skip ahead.

|                 |                                                                                |
|-----------------|--------------------------------------------------------------------------------|
| Not recommended | Read the reference guide if you'd like to learn more about custom dashboards.  |
| **Recommended** | If you'd like to learn more about custom dashboards, read the reference guide. |

### Oxford comma

The Oxford comma is the comma used after the second-to-last item in a list of three or more items. It appears just
before "and" or "or."

|                 |                                                                              |
|-----------------|------------------------------------------------------------------------------|
| Not recommended | Netdata can monitor RAM, disk I/O, MySQL queries per second and lm-sensors.  |
| **Recommended** | Netdata can monitor RAM, disk I/O, MySQL queries per second, and lm-sensors. |

### Future releases or features

Do not mention future releases or upcoming features in writing unless they have been previously communicated via a
public roadmap.

In particular, documentation must describe, as accurately as possible, the Netdata Agent _as of
the [latest commit](https://github.com/netdata/netdata/commits/master) in the GitHub repository_. For Netdata Cloud,
documentation must reflect the *current state* of [production](https://app.netdata.cloud).

### Informational links

Every link should clearly state its destination. Don't use words like "here" to describe where a link will take your
reader.

|                 |                                                                                                                                         |
|-----------------|-----------------------------------------------------------------------------------------------------------------------------------------|
| Not recommended | To install Netdata, click [here](https://github.com/netdata/netdata/blob/master/packaging/installer/README.md).                         |
| **Recommended** | To install Netdata, read the [installation instructions](https://github.com/netdata/netdata/blob/master/packaging/installer/README.md). |

Use links as often as required to provide necessary context. Blog posts and guides require less hyperlinks than
documentation. See the section on [linking between documentation](#linking-between-documentation) for guidance on the
Markdown syntax and path structure of inter-documentation links.

### Contractions

Contractions like "you'll" or "they're" are acceptable in most Netdata writing. They're both authentic and playful, and
reinforce the idea that you, as a writer, are guiding users through a particular idea, process, or feature.

Contractions are generally not used in press releases or other media engagements.

### Emoji

Emoji can add fun and character to your writing, but should be used sparingly and only if it matches the content's tone
and desired audience.

### Switching Linux users

Netdata documentation often suggests that users switch from their normal user to the `netdata` user to run specific
commands. Use the following command to instruct users to make the switch:

```bash
sudo su -s /bin/bash netdata
```

### Hostname/IP address of a node

Use `NODE` instead of an actual or example IP address/hostname when referencing the process of navigating to a dashboard
or API endpoint in a browser.

|                 |                                                                                                                                                                             |
|-----------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| Not recommended | Navigate to `http://example.com:19999` in your browser to see Netdata's dashboard. <br />Navigate to `http://203.0.113.0:19999` in your browser to see Netdata's dashboard. |
| **Recommended** | Navigate to `http://NODE:19999` in your browser to see Netdata's dashboard.                                                                                                 |

If you worry that `NODE` doesn't provide enough context for the user, particularly in documentation or guides designed
for beginners, you can provide an explanation:

> With the Netdata Agent running, visit `http://NODE:19999/api/v1/info` in your browser, replacing `NODE` with the IP
> address or hostname of your Agent.

### Paths and running commands

When instructing users to run a Netdata-specific command, don't assume the path to said command, because not every
Netdata Agent installation will have commands under the same paths. When applicable, help them navigate to the correct
path, providing a recommendation or instructions on how to view the running configuration, which includes the correct
paths.

For example, the [configuration](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md) doc first
teaches users how to find the Netdata config
directory and navigate to it, then runs commands from the `/etc/netdata` path so that the instructions are more
universal.

Don't include full paths, beginning from the system's root (`/`), as these might not work on certain systems.

|                 |                                                                                                                                                                                                                                                                                                    |
|-----------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| Not recommended | Use `edit-config` to edit Netdata's configuration: `sudo /etc/netdata/edit-config netdata.conf`.                                                                                                                                                                                                   |
| **Recommended** | Use `edit-config` to edit Netdata's configuration by first navigating to your [Netdata config directory](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md#the-netdata-config-directory), which is typically at `/etc/netdata`, then running `sudo edit-config netdata.conf`. |

### `sudo`

Include `sudo` before a command if you believe most Netdata users will need to elevate privileges to run it. This makes
our writing more universal, and users on `sudo`-less systems are generally already aware that they need to run commands
differently.

For example, most users need to use `sudo` with the `edit-config` script, because the Netdata config directory is owned
by the `netdata` user. Same goes for restarting the Netdata Agent with `systemctl`.

|                 |                                                                                                                                              |
|-----------------|----------------------------------------------------------------------------------------------------------------------------------------------|
| Not recommended | Run `edit-config netdata.conf` to configure the Netdata Agent. <br />Run `systemctl restart netdata` to restart the Netdata Agent.           |
| **Recommended** | Run `sudo edit-config netdata.conf` to configure the Netdata Agent. <br />Run `sudo systemctl restart netdata` to restart the Netdata Agent. |

## Markdown syntax

Netdata's documentation uses Markdown syntax.

If you're not familiar with Markdown, read the [Mastering
Markdown](https://guides.github.com/features/mastering-markdown/) guide from GitHub for the basics on creating
paragraphs, styled text, lists, tables, and more.

The following sections describe situations in which a specific syntax is required.

### Syntax standards (`remark-lint`)

The Netdata team uses [`remark-lint`](https://github.com/remarkjs/remark-lint) for Markdown code styling.

- Use a maximum of 120 characters per line.
- Begin headings with hashes, such as `# H1 heading`, `## H2 heading`, and so on.
- Use `_` for italics/emphasis.
- Use `**` for bold.
- Use dashes `-` to begin an unordered list, and put a single space after the dash.
- Tables should be padded so that pipes line up vertically with added whitespace.

If you want to see all the settings, open the
[`remarkrc.js`](https://github.com/netdata/netdata/blob/master/.remarkrc.js) file in the `netdata/netdata` repository.

### MDX and markdown

While writing in Docusaurus, you might want to take leverage of it's features that are supported in MDX formatted files.
One of those that we use is [Tabs](https://docusaurus.io/docs/next/markdown-features/tabs). They use an HTML syntax,
which requires some changes in the way we write markdown inside them.

In detail:

Due to a bug with docusaurus, we prefer to use `<h1>heading</h1> instead of # H1` so that docusaurus doesn't render the
contents of all Tabs on the right hand side, while not being able to navigate
them [relative link](https://github.com/facebook/docusaurus/issues/7008).

You can use markdown syntax for every other styling you want to do except Admonitions:
For admonitions, follow [this](https://docusaurus.io/docs/markdown-features/admonitions#usage-in-jsx) guide to use
admonitions inside JSX. While writing in JSX, all the markdown stylings have to be in HTML format to be rendered
properly.

### Frontmatter

Every document must begin with frontmatter, followed by an H1 (`#`) heading.

Unlike typical Markdown frontmatter, Netdata uses HTML comments (`<!--`, `-->`) to begin and end the frontmatter block.
These HTML comments are later converted into typical frontmatter syntax when building [Netdata
Learn](https://learn.netdata.cloud).

Frontmatter *must* contain the following variables:

- A `title` that quickly and distinctly describes the document's content.
- A `description` that elaborates on the purpose or goal of the document using no less than 100 characters and no more
  than 155 characters.
- A `custom_edit_url` that links directly to the GitHub URL where another user could suggest additional changes to the
  published document.

Some documents, like the Ansible guide and others in the `/docs/guides` folder, require an `image` variable as well. In
this case, replace `/docs` with `/img/seo`, and then rebuild the remainder of the path to the document in question. End
the path with `.png`. A member of the Netdata team will assist in creating the image when publishing the content.

For example, here is the frontmatter for the guide
about [deploying the Netdata Agent with Ansible](https://github.com/netdata/netdata/blob/master/packaging/installer/methods/ansible.md).

<img width="751" alt="image" src="https://user-images.githubusercontent.com/43294513/217607958-ef0f270d-7947-4d91-a9a5-56b17b4255ee.png">


Questions about frontmatter in documentation? [Ask on our community
forum](https://community.netdata.cloud/c/blog-posts-and-articles/6).

### Admonitions

In addition to basic markdown syntax, we also encourage the use of admonition syntax, which allows for a more
aesthetically seamless presentation of supplemental information. For general instructions on using admonitions, feel
free to read this [feature guide](https://docusaurus.io/docs/markdown-features/admonitions).

We encourage the use of **Note** admonitions to provide important supplemental information to a user within a task step,
reference item, or concept passage.

Additionally, you should use a **Caution** admonition to provide necessary information to present any risk to a user's
setup or data.

**Danger** admonitions should be avoided, as these admonitions are typically applied to reduce physical or bodily harm
to an individual.

### Linking between documentation

Documentation should link to relevant pages whenever it's relevant and provides valuable context to the reader.

We link between markdown documents by using its GitHub absolute link for 
instance `[short description of what we reference](https://github.com/netdata/netdata/blob/master/contribution-guidelines.md)`

### References to UI elements

When referencing a user interface (UI) element in Netdata, reference the label text of the link/button with Markdown's
(`**bold text**`) tag.

```markdown
Click the **Sign in** button.
```

Avoid directional language whenever possible. Not every user can use instructions like "look at the top-left corner" to
find their way around an interface, and interfaces often change between devices. If you must use directional language,
try to supplement the text with an [image](#images).

### Images

Don't rely on images to convey features, ideas, or instructions. Accompany every image with descriptive alt text.

In Markdown, use the standard image syntax, `![](/docs/agent/contributing)`, and place the alt text between the
brackets `[]`. Here's an example
using our logo:

```markdown
![The Netdata logo](/docs/agent/web/gui/static/img/netdata-logomark.svg)
```

Reference in-product text, code samples, and terminal output with actual text content, not screen captures or other
images. Place the text in an appropriate element, such as a blockquote or code block, so all users can parse the
information.

### Syntax highlighting

Our documentation site at [learn.netdata.cloud](https://learn.netdata.cloud) uses
[Prism](https://v2.docusaurus.io/docs/markdown-features#syntax-highlighting) for syntax highlighting. Netdata
documentation will use the following for the most part: `c`, `python`, `js`, `shell`, `markdown`, `bash`, `css`, `html`,
and `go`. If no language is specified, Prism tries to guess the language based on its content.

Include the language directly after the three backticks (```` ``` ````) that start the code block. For highlighting C
code, for example:

````c
```c
inline char *health_stock_config_dir(void) {
    char buffer[FILENAME_MAX + 1];
    snprintfz(buffer, FILENAME_MAX, "%s/health.d", netdata_configured_stock_config_dir);
    return config_get(CONFIG_SECTION_DIRECTORIES, "stock health config", buffer);
}
```
````

And the prettified result:

```c
inline char *health_stock_config_dir(void) {
    char buffer[FILENAME_MAX + 1];
    snprintfz(buffer, FILENAME_MAX, "%s/health.d", netdata_configured_stock_config_dir);
    return config_get(CONFIG_SECTION_DIRECTORIES, "stock health config", buffer);
}
```

Prism also supports titles and line highlighting. See
the [Docusaurus documentation](https://v2.docusaurus.io/docs/markdown-features#code-blocks) for more information.

## Word list

The following tables describe the standard spelling, capitalization, and usage of words found in Netdata's writing.

### Netdata-specific terms

| Term                                               | Definition                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                     |
|----------------------------------------------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| **claimed node**                                   | A node that you've proved ownership of by completing the [connecting to Cloud process](https://github.com/netdata/netdata/blob/master/claim/README.md). The claimed node will then appear in your Space and any War Rooms you added it to.                                                                                                                                                                                                                                                                                                                                     |
| **Netdata**                                        | The company behind the open-source Netdata Agent and the Netdata Cloud web application. Never use *netdata* or *NetData*. <br /><br />**Note:** You should use "Netdata" when referencing any general element, function, or part of the user experience. In general, focus on the user's goals, actions, and solutions rather than what the company provides.  For example, write *Learn more about enabling alarm notifications on your preferred platforms* instead of *Netdata sends alarm notifications to your preferred platforms*.                                      |
| **Netdata Agent** or **Open-source Netdata Agent** | The free and open source [monitoring agent](https://github.com/netdata/netdata) that you can install on all of your distributed systems, whether they're physical, virtual, containerized, ephemeral, and more. The Agent monitors systems running Linux, Docker, Kubernetes, macOS, FreeBSD, and more, and collects metrics from hundreds of popular services and applications. <br /><br /> **Note:** You should avoid referencing the Netdata Agent or Open-source Netdata agent in any scenario that does not specifically require the distinction for clear instructions. |
| **Netdata Cloud**                                  | The web application hosted at [https://app.netdata.cloud](https://app.netdata.cloud) that helps you monitor an entire infrastructure of distributed systems in real time. <br /><br />**Notes:** Never use *Cloud* without the preceding *Netdata* to avoid ambiguity. You should avoid referencing Netdata Cloud in any scenario that does not specifically require the distinction for clear instructions.                                                                                                                                                                   |                                                                                                                                                      |
| **Netdata community**                              | Contributors to any of Netdata's [open-source projects](https://github.com/netdata/learn/blob/master/contribute/projects.md), members of the [community forum](https://community.netdata.cloud/).                                                                                                                                                                                                                                                                                                                                                                                                                             |
| **Netdata community forum**                        | The Discourse-powered forum for feature requests, Netdata Cloud technical support, and conversations about Netdata's monitoring and troubleshooting products.                                                                                                                                                                                                                                                                                                                                                                                                                  |
| **node**                                           | A system on which the Netdata Agent is installed. The system can be physical, virtual, in a Docker container, and more. Depending on your infrastructure, you may have one, dozens, or hundreds of nodes. Some nodes are *ephemeral*, in that they're created/destroyed automatically by an orchestrator service.                                                                                                                                                                                                                                                              |
| **Space**                                          | The highest level container within Netdata Cloud for a user to organize their team members and nodes within their infrastructure. A Space likely represents an entire organization or a large team. <br /><br />*Space* is always capitalized.                                                                                                                                                                                                                                                                                                                                 |
| **unreachable node**                               | A connected node with a disrupted [Agent-Cloud link](https://github.com/netdata/netdata/blob/master/aclk/README.md). Unreachable could mean the node no longer exists or is experiencing network connectivity issues with Cloud.                                                                                                                                                                                                                                                                                                                                               |
| **visited node**                                   | A node which has had its Agent dashboard directly visited by a user. A list of these is maintained on a per-user basis.                                                                                                                                                                                                                                                                                                                                                                                                                                                        |
| **War Room**                                       | A smaller grouping of nodes where users can view key metrics in real-time and monitor the health of many nodes with their alarm status. War Rooms can be used to organize nodes in any way that makes sense for your infrastructure, such as by a service, purpose, physical location, and more.  <br /><br />*War Room* is always capitalized.                                                                                                                                                                                                                                |

### Other technical terms

| Term                        | Definition                                                                                                                                                                                                                  |
|-----------------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| **filesystem**              | Use instead of *file system*.                                                                                                                                                                                               |
| **preconfigured**           | The concept that many of Netdata's features come with sane defaults that users don't need to configure to find [immediate value](/docs/overview/why-netdata#simple-to-deploy).                                              |
| **real time**/**real-time** | Use *real time* as a noun phrase, most often with *in*: *Netdata collects metrics in real time*. Use *real-time* as an adjective: _Netdata collects real-time metrics from hundreds of supported applications and services. |
