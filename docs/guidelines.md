<!--
title: "Contribute to the documentation"
sidebar_label: "Contribute to the documentation"
custom_edit_url: "https://github.com/netdata/netdata/blob/master/docs/guidelines.md"
sidebar_position: "10"
learn_status: "Published"
learn_topic_type: "Custom"
learn_rel_path: "Contribute"
-->

import Tabs from '@theme/Tabs'; import TabItem from '@theme/TabItem';

Welcome to our docs developer guidelines!

This document will guide you to the process of contributing to our
docs (**learn.netdata.cloud**)

## Documentation architecture

Netdata docs follows has two principals.

1. Keep the documentation of each component _as close as you can to the codebase_
2. Every component is analyzed via topic related docs.

To this end:

1. Documentation lives in every possible repo in the netdata organization. At the moment we contribute to:
    - netdata/netdata
    - netdata/learn (final site)
    - netdata/go.d.plugin
    - netdata/agent-service-discovery

   In each of these repos you will find markdown files. These markdown files may or not be part of the final docs. You
   understand what documents are part of the final docs in the following section:[_How to update documentation of
   learn.netdata.cloud_](#how-to-update-documentation-of-learn-netdata-cloud)

2. Netdata docs processes are inspired from
   the [DITA 1.2 guidelines](http://docs.oasis-open.org/dita/v1.2/os/spec/archSpec/dita-1.2_technicalContent_overview.html)
   for Technical content.

## Topic types

### Concepts

A concept introduces a single feature or concept. A concept should answer the questions:

- What is this?
- Why would I use it?

Concept topics:

- Are abstract ideas
- Explain meaning or benefit
- Can stay when specifications change
- Provide background information

### Tasks

Concept and reference topics exist to support tasks. _The goal for users … is not to understand a concept but to
complete a task_. A task gives instructions for how to complete a procedure.

Much of the uncertainty whether a topic is a concept or a reference disappears, when you have strong, solid task topics
in place, furthermore topics directly address your users and their daily tasks and help them to get their job done. A
task **must give an answer** to the **following questions**:

- How do I create cool espresso drinks with my new coffee machine?
- How do I clean the milk steamer?

For the title text, use the structure active verb + noun. For example, for instance _Deploy the Agent_.

### References

The reference document and information types provide for the separation of fact-based information from concepts and
tasks. \
Factual information may include tables and lists of specifications, parameters, parts, commands, edit-files and other
information that the users are likely to look up. The reference information type allows fact-based content to be
maintained by those responsible for its accuracy and consistency.

## Contribute to the documentation of learn.netdata.cloud

### Encapsulate topics into markdown files.

Netdata uses markdown files to document everything. To implement concrete sections of these [Topic types](#topic-types)
we encapsulate this logic as follows. Every document is characterized by its topic type ('learn_topic_type' metadata
field). To avoid breaking every single netdata concept into numerous small markdown files each document can be either a
single `Reference` or `Concept` or `Task` or a group of `References`, `Concepts`, `Tasks`.

To this end, every single topic is encapsulated into a `Heading 3 (###)` section. That means, when you have a single
file you only make use of `Headings 4` and lower (`4, 5, 6`, for templated section or subsection). In case you want to
includ multiple (`Concepts` let's say) in a single document, you use `Headings 3` to seperate each concept. `Headings 2`
are used only in case you want to logically group topics inside a document.

For instance:

```markdown

Small introduction of the document.

### Concept A

Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor incididunt ut labore et dolore magna
aliqua.

#### Field from template 1

Ut enim ad minim veniam, quis nostrud exercitation ullamco laboris nisi ut aliquip ex ea commodo consequat.

#### Field from template 1

Duis aute irure dolor in reprehenderit in voluptate velit esse cillum dolore eu fugiat nulla pariatur.

##### Subsection 1

. . .

### Concept A

Excepteur sint occaecat cupidatat non proident, sunt in culpa qui officia deserunt mollit anim id est laborum.

#### Field from template 1

. . .


```

This approach gives a clean and readable outlook in each document from a single sidebar.

Here you can find the preferred templates for each topic type:


<Tabs>
  <TabItem value="Concept" label="Concept" default>

   ```markdown
    Small intro, give some context to the user of what you will cover on this document

    ### concept title (omit if the document describes only one concept)
    
    A concept introduces a single feature or concept. A concept should answer the questions:
    
    1. What is this?
    2. Why would I use it?

   ```

  </TabItem>
  <TabItem value="Task" label="Tasks">

   ```markdown
    Small intro, give some context to the user of what you will cover on this document
    
    ### Task title (omit if the document describes only one task)

    #### Prerequisite
    
    Unordered list of what you will need. 
    
    #### Steps
    
    Exact list of step the user must follow
    
    #### Expected result
    
    What you expect to see when you complete the steps above
    
    #### Example
    
    Example configuration/actions of the task
    
    #### Related reference documentation
    
    List of reference docs user needs to be aware of.
   ```

  </TabItem>
  <TabItem value="Reference-collectors" label="Reference-collectors">

   ```markdown
    Small intro, give some context to the user of what you will cover on this document

    ### Reference name (omit if the document describes only one reference)

    #### Requirements
    
    Document any dependencies needed to run this module

    #### Requirements on the monitored component

    Document any steps user must take to sucessful monitor application,
    for instance (create a user)

    #### Configuration files

    table with path and configuration files purpose
    Columns: File name | Description (Purpose in a nutshell)

    #### Data collection

    To make changes, see `the ./edit-config task <link>`
    
    #### Auto discovery
    
    ##### Single node installation 
    
    . . . we autodetect localhost:port and what configurations are defaults 
    
    ##### Kubernetes installations
    
    . . . Service discovery, click here
    
    #### Metrics
    
    Columns: Metric (Context) | Scope | description (of the context) | dimensions | units (of the context)  | Alert triggered

    
    #### Alerts 
    
    Collapsible content for every alert, just like the alert guides

    #### Configuration options
    
    Table with all the configuration options available. 
    
    Columns: name | description | default | file_name
    
    #### Configuration example

    Default configuration example
 
    #### Troubleshoot
    
    backlink to the task to run this module in debug mode (here you provide the debug flags)


```

  </TabItem>
</Tabs>

### Metadata fields

All Docs that are supposed to be part of learn.netdata.cloud have **hidden** sections in the begining of document. These
sections are plain lines of text and we call them metadata. Their represented as `key : "Value"` pairs. Some of them are
needed from our statice website builder (docusaurus) others are needed for our internal pipelines to build docs
(have prefix `learn_`).

So let's go through the different necessary metadata tags to get a document properly published on Learn:

|     metadata_key      | Value(s)                                                                                                      |                                                                     Frontmatter effect                                                                      | Mandatory |               Limitations               |
|:---------------------:|---------------------------------------------------------------------------------------------------------------|:-----------------------------------------------------------------------------------------------------------------------------------------------------------:|:---------:|:---------------------------------------:|
|        `title`        | `String`                                                                                                      |                                                                   Title in each document                                                                    |    yes    |                                         |
|   `custom_edit_url`   | `String`                                                                                                      |                                                               The source GH link of the file                                                                |    yes    |                                         |
|     `description`     | `String or multiline String`                                                                                  |                                                                              -                                                                              |    yes    |                                         |
|    `sidebar_label`    | `String or multiline String`                                                                                  |                                                                    Name in the TOC tree                                                                     |    yes    |                                         |
|  `sidebar_position`   | `String or multiline String`                                                                                  |                                                   Global position in the TOC tree (local for per folder)                                                    |    yes    |                                         |
|    `learn_status`     | [`Published`, `Unpublished`, `Hidden`]                                                                        | `Published`: Document visible in learn,<br/> `Unpublished`: Document archived in learn, <br/>`Hidden`: Documentplaced under learn_rel_path but it's hidden] |    yes    |                                         |
|  `learn_topic_type`   | [`Concepts`, `Tasks`, `References`, `Getting Started`]                                                        |                                                                                                                                                             |    yes    |                                         |
|   `learn_rel_path`    | `Path` (the path you want this file to appear in learn<br/> without the /docs prefix and the name of the file |                                                                                                                                                             |    yes    |                                         |
| `learn_autogenerated` | `Dictionary` (for internal use)                                                                               |                                                                                                                                                             |    no     | Keys in the dictionary must be in `' '` |

:::important

1. In case any mandatory tags are missing or falsely inputted the file will remain unpublished. This is by design to
   prevent non-properly tagged files from getting published.
2. All metadata values must be included in `" "`. From `string` noted text inside the fields use `' ''`


While Docusaurus can make use of more metadata tags than the above, these are the minimum we require to publish the file
on Learn.

:::

### Placing a document in learn

Here you can see how the metadata are parsed and create a markdown file in learn.

![](https://user-images.githubusercontent.com/12612986/207310336-f7cc150b-543c-4f13-be98-5058a4d29284.png)

### Before you get started

Anyone interested in contributing to documentation should first read the [Netdata style guide](#styling-guide) further
down below and the [Netdata Community Code of Conduct](https://github.com/netdata/.github/blob/main/CODE_OF_CONDUCT.md).

Netdata's documentation uses Markdown syntax. If you're not familiar with Markdown, read
the [Mastering Markdown](https://guides.github.com/features/mastering-markdown/) guide from GitHub for the basics on
creating paragraphs, styled text, lists, tables, and more, and read further down about some special
occasions [while writing in MDX](#mdx-and-markdown).

### Making your first contribution

The easiest way to contribute to Netdata's documentation is to edit a file directly on GitHub. This is perfect for small
fixes to a single document, such as fixing a typo or clarifying a confusing sentence.

Click on the **Edit this page** button on any published document on [Netdata Learn](https://learn.netdata.cloud). Each
page has two of these buttons: One beneath the table of contents, and another at the end of the document, which take you
to GitHub's code editor. Make your suggested changes, keeping the [Netdata style guide](#styling-guide)
in mind, and use the ***Preview changes*** button to ensure your Markdown syntax works as expected.

Under the **Commit changes**  header, write descriptive title for your requested change. Click the **Commit changes**
button to initiate your pull request (PR).

Jump down to our instructions on [PRs](#making-a-pull-request) for your next steps.

**Note**: If you wish to contribute documentation that is more tailored from your specific infrastructure
monitoring/troubleshooting experience, please consider submitting a blog post about your experience. Check
the [README](https://github.com/netdata/blog/blob/master/README.md) in our blog repo! Any blog submissions that have
widespread or universal application will be integrated into our permanent documentation.

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

## Styling guide

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

If you're not familiar with Markdown, read
the [Mastering Markdown](https://guides.github.com/features/mastering-markdown/) guide from GitHub for the basics on
creating paragraphs, styled text, lists, tables, and more.

The following sections describe situations in which a specific syntax is required.

#### Syntax standards (`remark-lint`)

The Netdata team uses [`remark-lint`](https://github.com/remarkjs/remark-lint) for Markdown code styling.

- Use a maximum of 120 characters per line.
- Begin headings with hashes, such as `# H1 heading`, `## H2 heading`, and so on.
- Use `_` for italics/emphasis.
- Use `**` for bold.
- Use dashes `-` to begin an unordered list, and put a single space after the dash.
- Tables should be padded so that pipes line up vertically with added whitespace.

If you want to see all the settings, open the
[`remarkrc.js`](https://github.com/netdata/netdata/blob/master/.remarkrc.js) file in the `netdata/netdata` repository.

#### MDX and markdown

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

#### Admonitions

Use admonitions cautiously. Admonitions may draw user's attention, to that end we advise you to use them only for side
content/info, without significantly interrupting the document flow.

You can find the supported admonitions in the docusaurus's [documentation](https://docusaurus.io/docs/markdown-features/admonitions).

#### Images

Don't rely on images to convey features, ideas, or instructions. Accompany every image with descriptive alt text.

In Markdown, use the standard image syntax, `![](/docs/agent/contributing)`, and place the alt text between the
brackets `[]`. Here's an example using our logo:

```markdown
![The Netdata logo](/docs/agent/web/gui/static/img/netdata-logomark.svg)
```

Reference in-product text, code samples, and terminal output with actual text content, not screen captures or other
images. Place the text in an appropriate element, such as a blockquote or code block, so all users can parse the
information.

#### Syntax highlighting

Our documentation site at [learn.netdata.cloud](https://learn.netdata.cloud) uses
[Prism](https://v2.docusaurus.io/docs/markdown-features#syntax-highlighting) for syntax highlighting. Netdata can use
any of
the [supported languages by prism-react-renderer](https://github.com/FormidableLabs/prism-react-renderer/blob/master/src/vendor/prism/includeLangs.js)
.

If no language is specified, Prism tries to guess the language based on its content.

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

## Language, grammar, and mechanics

#### Voice and tone

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

##### Voice

Netdata's voice is authentic, passionate, playful, and respectful.

- **Authentic** writing is honest and fact-driven. Focus on Netdata's strength while accurately communicating what
  Netdata can and cannot do, and emphasize technical accuracy over hard sells and marketing jargon.
- **Passionate** writing is strong and direct. Be a champion for the product or feature you're writing about, and let
  your unique personality and writing style shine.
- **Playful** writing is friendly, thoughtful, and engaging. Don't take yourself too seriously, as long as it's not at
  the expense of Netdata or any of its users.
- **Respectful** writing treats people the way you want to be treated. Prioritize giving solutions and answers as
  quickly as possible.

##### Tone

Netdata's tone is fun and playful, but clarity and conciseness comes first. We also tend to be informal, and aren't
afraid of a playful joke or two.

While we have general standards for voice and tone, we do want every individual's unique writing style to reflect in
published content.

#### Universal communication

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

To ensure Netdata's writing is clear, concise, and universal, we have established standards for language, grammar, and
certain writing mechanics. However, if you're writing about Netdata for an external publication, such as a guest blog
post, follow that publication's style guide or standards, while keeping
the [preferred spelling of Netdata terms](#netdata-specific-terms) in mind.

#### Active voice

Active voice is more concise and easier to understand compared to passive voice. When using active voice, the subject of
the sentence is action. In passive voice, the subject is acted upon. A famous example of passive voice is the phrase
"mistakes were made."

|                 |                                                                                           |
| --------------- | ----------------------------------------------------------------------------------------- |
| Not recommended | When an alarm is triggered by a metric, a notification is sent by Netdata.                |
| **Recommended** | When a metric triggers an alarm, Netdata sends a notification to your preferred endpoint. |

#### Second person

Use the second person ("you") to give instructions or "talk" directly to users.

In these situations, avoid "we," "I," "let's," and "us," particularly in documentation. The "you" pronoun can also be
implied, depending on your sentence structure.

One valid exception is when a member of the Netdata team or community wants to write about said team or community.

|                                |                                                              |
| ------------------------------ | ------------------------------------------------------------ |
| Not recommended                | To install Netdata, we should try the one-line installer...  |
| **Recommended**                | To install Netdata, you should try the one-line installer... |
| **Recommended**, implied "you" | To install Netdata, try the one-line installer...            |

#### "Easy" or "simple"

Using words that imply the complexity of a task or feature goes against our policy
of [universal communication](#universal-communication). If you claim that a task is easy and the reader struggles to
complete it, you may inadvertently discourage them.

However, if you give users two options and want to relay that one option is genuinely less complex than another, be
specific about how and why.

For example, don't write, "Netdata's one-line installer is the easiest way to install Netdata." Instead, you might want
to say, "Netdata's one-line installer requires fewer steps than manually installing from source."

#### Slang, metaphors, and jargon

A particular word, phrase, or metaphor you're familiar with might not translate well to the other cultures featured
among Netdata's global community. We recommended you avoid slang or colloquialisms in your writing.

In addition, don't use abbreviations that have not yet been defined in the content. See our section on
[abbreviations](#abbreviations-acronyms-and-initialisms) for additional guidance.

If you must use industry jargon, such as "mean time to resolution," define the term as clearly and concisely as you can.

> Netdata helps you reduce your organization's mean time to resolution (MTTR), which is the average time the responsible
> team requires to repair a system and resolve an ongoing incident.

#### Spelling

While the Netdata team is mostly *not* American, we still aspire to use American spelling whenever possible, as it is
the standard for the monitoring industry.

See the [word list](#word-list) for spellings of specific words.

#### Capitalization

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
| --------------- | ---------------------------------------------------------------------------------------------- |
| Not recommended | The netdata agent, which spawns the netdata process, is actively maintained by netdata, inc.   |
| **Recommended** | The Netdata Agent, which spawns the `netdata` process, is actively maintained by Netdata, Inc. |

##### Capitalization of document titles and page headings

Document titles and page headings should use sentence case. That means you should only capitalize the first word.

If you need to use the name of a brand, software, product, and company, capitalize it according to their official
guidelines.

Also, don't put a period (`.`) or colon (`:`) at the end of a title or header.

|                 |                                                                                                     |
| --------------- | --------------------------------------------------------------------------------------------------- |
| Not recommended | Getting Started Guide <br />Service Discovery and Auto-Detection: <br />Install netdata with docker |
| **
Recommended** | Getting started guide <br />Service discovery and auto-detection <br />Install Netdata with Docker  |

#### Abbreviations (acronyms and initialisms)

Use abbreviations (including [acronyms and initialisms](https://www.dictionary.com/e/acronym-vs-abbreviation/)) in
documentation when one exists, when it's widely accepted within the monitoring/sysadmin community, and when it improves
the readability of a document.

When introducing an abbreviation to a document for the first time, give the reader both the spelled-out version and the
shortened version at the same time. For example:

> Use Netdata to monitor Extended Berkeley Packet Filter (eBPF) metrics in real-time. After you define an abbreviation, don't switch back and forth. Use only the abbreviation for the rest of the document.

You can also use abbreviations in a document's title to keep the title short and relevant. If you do this, you should
still introduce the spelled-out name alongside the abbreviation as soon as possible.

#### Clause order

When instructing users to take action, give them the context first. By placing the context in an initial clause at the
beginning of the sentence, users can immediately know if they want to read more, follow a link, or skip ahead.

|                 |                                                                                |
| --------------- | ------------------------------------------------------------------------------ |
| Not recommended | Read the reference guide if you'd like to learn more about custom dashboards.  |
| **Recommended** | If you'd like to learn more about custom dashboards, read the reference guide. |

#### Oxford comma

The Oxford comma is the comma used after the second-to-last item in a list of three or more items. It appears just
before "and" or "or."

|                 |                                                                              |
| --------------- | ---------------------------------------------------------------------------- |
| Not recommended | Netdata can monitor RAM, disk I/O, MySQL queries per second and lm-sensors.  |
| **Recommended** | Netdata can monitor RAM, disk I/O, MySQL queries per second, and lm-sensors. |

#### Future releases or features

Do not mention future releases or upcoming features in writing unless they have been previously communicated via a
public roadmap.

In particular, documentation must describe, as accurately as possible, the Netdata Agent _as of
the [latest commit](https://github.com/netdata/netdata/commits/master) in the GitHub repository_. For Netdata Cloud,
documentation must reflect the *current state* of [production](https://app.netdata.cloud).

#### Informational links

Every link should clearly state its destination. Don't use words like "here" to describe where a link will take your
reader.

|                 |                                                                                            |
| --------------- | ------------------------------------------------------------------------------------------ |
| Not recommended | To install Netdata, click [here](https://github.com/netdata/netdata/blob/master/packaging/installer/README.md).                         |
| **Recommended** | To install Netdata, read the [installation instructions](https://github.com/netdata/netdata/blob/master/packaging/installer/README.md). |

Use links as often as required to provide necessary context. Blog posts and guides require less hyperlinks than
documentation. See the section on [linking between documentation](#linking-between-documentation) for guidance on the
Markdown syntax and path structure of inter-documentation links.

#### Contractions

Contractions like "you'll" or "they're" are acceptable in most Netdata writing. They're both authentic and playful, and
reinforce the idea that you, as a writer, are guiding users through a particular idea, process, or feature.

Contractions are generally not used in press releases or other media engagements.

#### Emoji

Emoji can add fun and character to your writing, but should be used sparingly and only if it matches the content's tone
and desired audience.

#### Switching Linux users

Netdata documentation often suggests that users switch from their normal user to the `netdata` user to run specific
commands. Use the following command to instruct users to make the switch:

```bash
sudo su -s /bin/bash netdata
```

#### Hostname/IP address of a node

Use `NODE` instead of an actual or example IP address/hostname when referencing the process of navigating to a dashboard
or API endpoint in a browser.

|                 |                                                                                                                                                                             |
| --------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Not recommended | Navigate to `http://example.com:19999` in your browser to see Netdata's dashboard. <br />Navigate to `http://203.0.113.0:19999` in your browser to see Netdata's dashboard. |
| **
Recommended** | Navigate to `http://NODE:19999` in your browser to see Netdata's dashboard.                                                                                                 |

If you worry that `NODE` doesn't provide enough context for the user, particularly in documentation or guides designed
for beginners, you can provide an explanation:

> With the Netdata Agent running, visit `http://NODE:19999/api/v1/info` in your browser, replacing `NODE` with the IP
> address or hostname of your Agent.

#### Paths and running commands

When instructing users to run a Netdata-specific command, don't assume the path to said command, because not every
Netdata Agent installation will have commands under the same paths. When applicable, help them navigate to the correct
path, providing a recommendation or instructions on how to view the running configuration, which includes the correct
paths.

For example, the [configuration](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md) doc first teaches users how to find the Netdata config directory
and navigate to it, then runs commands from the `/etc/netdata` path so that the instructions are more universal.

Don't include full paths, beginning from the system's root (`/`), as these might not work on certain systems.

|                 |                                                                                                                                                                                                                                                   |
| --------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Not recommended | Use `edit-config` to edit Netdata's configuration: `sudo /etc/netdata/edit-config netdata.conf`.                                                                                                                                                  |
| **
Recommended** | Use `edit-config` to edit Netdata's configuration by first navigating to your [Netdata config directory](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md#the-netdata-config-directory), which is typically at `/etc/netdata`, then running `sudo edit-config netdata.conf`. |

#### `sudo`

Include `sudo` before a command if you believe most Netdata users will need to elevate privileges to run it. This makes
our writing more universal, and users on `sudo`-less systems are generally already aware that they need to run commands
differently.

For example, most users need to use `sudo` with the `edit-config` script, because the Netdata config directory is owned
by the `netdata` user. Same goes for restarting the Netdata Agent with `systemctl`.

|                 |                                                                                                                                              |
| --------------- | -------------------------------------------------------------------------------------------------------------------------------------------- |
| Not recommended | Run `edit-config netdata.conf` to configure the Netdata Agent. <br />Run `systemctl restart netdata` to restart the Netdata Agent.           |
| **
Recommended** | Run `sudo edit-config netdata.conf` to configure the Netdata Agent. <br />Run `sudo systemctl restart netdata` to restart the Netdata Agent. |

## Deploy and test docs

<!--
TODO: Update this section after implemeting a _docker-compose_ for builting and testing learn 
-->

The Netdata team aggregates and publishes all documentation at [learn.netdata.cloud](/) using
[Docusaurus](https://v2.docusaurus.io/) over at the [`netdata/learn` repository](https://github.com/netdata/learn).

## Netdata-specific terms

Consult the [Netdata Glossary](https://github.com/netdata/netdata/blob/master/docs/glossary.md) Netdata specific terms 
