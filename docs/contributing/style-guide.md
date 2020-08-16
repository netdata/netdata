<!--
title: "Netdata style guide"
description: "Learn about Netdata's writing and Markdown standards for documentation and educational content."
custom_edit_url: https://github.com/netdata/netdata/edit/master/docs/contributing/style-guide.md
-->

# Netdata style guide

This in-progress style guide establishes editorial guidelines for anyone who wants to write documentation for Netdata
products.

## Table of contents

-   [Welcome!](#welcome)
-   [Goals of the Netdata style guide](#goals-of-the-Netdata-style-guide)
-   [General principles](#general-principles)
-   [Tone and content](#tone-and-content)
-   [Language and grammar](#language-and-grammar)
-   [Markdown syntax](#markdown-syntax)
-   [Accessibility](#accessibility)

## Welcome

Proper documentation is essential to the success of any open-source project. Netdata is no different. The health of our
monitoring agent and our community depends on this effort.

We're here to make developers, sysadmins, and DevOps engineers better at their jobs, after all!

We welcome contributions to Netdata's documentation. Begin with the [contributing to documentation
guide](contributing-documentation.md), followed by this style guide.

## Goals of the Netdata style guide

An editorial style guide establishes standards for writing and maintaining documentation. At Netdata, we focus on the
following principles:

-   Consistency
-   High-quality writing
-   Conciseness
-   Accessibility

These principles will make the documentation better for everyone who wants to use Netdata, whether they're a beginner or
an expert.

### Breaking the rules

None of the rules described in this style guide are absolute. **We welcome rule-breaking if it creates better, more
accessible documentation.**

But be aware that Netdata staff or community members may ask you to justify your rule-breaking during the PR review
process.

## General principles

Yes, this style guide is pretty overwhelming! Establishing standards for a global community is never easy.

Let's start with a few key points to start. Where relevant, these points link to more in-depth information about a given
rule.

**[Tone and content](#tone-and-content)**:

-   Be [conversational and friendly](#conversational-and-friendly-tone).
-   Write [concisely](#write-concisely).
-   Don't use words like **here** when [creating hyperlinks](#use-informational-hyperlinks).
-   Don't mention [future releases or features](#mentioning-future-releases-or-features) in the documentation.

**[Language and grammar](#language-and-grammar)**:

-   [Capitalize words](#capitalization) at the beginning of sentences, for proper nouns, and at the beginning of
    document titles and section headers.
-   Use [second person](#second-person)—"you" rather than "we"—when giving instructions.
-   Use [active voice](#active-voice) to make clear who or what is acting.
-   Always employ an [Oxford comma](#oxford-comma) on lists.

**[Markdown syntax](#markdown-syntax)**:

-   [Reference UI elements](#references-to-ui-elements) with bold text.
-   Use our [built-in syntax highlighter](#language-specific-syntax-highlighting-in-code-blocks) to improve the
    readability and usefulness of code blocks.

**[Accessibility](#accessibility)**:

-   Include [alt tags on images](#images).

---

## Tone and content

Netdata's documentation should be conversational, concise, and informational, without feeling overly formal.

By following a few principles on tone and content we'll ensure more readers from every background and skill level will
learn as much as possible about Netdata's capabilities.

### Conversational and friendly tone

Netdata's documentation should be conversational and friendly. To borrow from Google's fantastic [developer style
guide](https://developers.google.com/style/tone):

> Try to sound like a knowledgeable friend who understands what the developer wants to do.

Feel free to let some of your personality show! Documentation can be highly professional without being dry, formal, or
overly instructive.

### Write concisely

You should always try to use as few words as possible to explain a particular feature, configuration, or process.
Conciseness leads to more accurate and understandable writing.

### Use informational hyperlinks

Hyperlinks should clearly state its destination. Don't use words like "here" to describe where a link will take your
reader.

```markdown
# Not recommended
To install Netdata, click [here](/packaging/installer/README.md).

# Recommended
To install Netdata, read our [installation instructions](/packaging/installer/README.md).
```

In general, guides should include fewer hyperlinks to keep the reader focused on the task at hand. Documentation should
include as many hyperlinks as necessary to provide meaningful context.

### Avoid words like "easy" or "simple"

Never assume readers of Netdata documentation are experts in Netdata's inner workings or health monitoring/performance
troubleshooting in general.

If you claim that a task is easy and the reader struggles to complete it, you may inadvertently discourage them.

If you perceive one option to be easier than another, be specific about how and why. For example, don't write,
"Netdata's one-line installer is the easiest way to install Netdata." Instead, you might want to say, "Netdata's
one-line installer requires fewer steps than manually installing from source."

### Avoid slang, metaphors, and jargon

A particular word, phrase, or metaphor you're familiar with might not translate well to the other cultures featured
among Netdata's global community. We recommended you avoid slang or colloquialisms in your writing.

In addition, don't use abbreviations that have not yet been defined in the document. See our section on
[abbreviations](#abbreviations-acronyms-and-initialisms) for more information.

If you must use industry jargon, such as "white-box monitoring," in a document, define the term as clearly and concisely
as you can.

> White-box monitoring: Monitoring of a system or application based on the metrics it directly exposes, such as logs.

Avoid emojis whenever possible for the same reasons—they can be difficult to understand immediately and don't translate
well.

### Mentioning future releases or features

Documentation is meant to describe the product as-is, not as it will be or could be in the future. Netdata documentation
generally avoids talking about future features or products, even if we know they are inevitable.

An exception can be made for documenting beta features that are subject to change with further development.

## Language and grammar

Netdata's documentation should be consistent in the way it uses certain words, phrases, and grammar. The following
sections will outline the preferred usage for capitalization, point of view, active voice, and more.

### Capitalization

Follow the general [English standards](https://owl.purdue.edu/owl/general_writing/mechanics/help_with_capitals.html) for
capitalization. In summary:

-   Capitalize the first word of every new sentence.
-   Don't use uppercase for emphasis. (Netdata is the BEST!)
-   Capitalize the names of brands, software, products, and companies according to their official guidelines. (Netdata,
    Docker, Apache, NGINX)
-   Avoid camel case (NetData) or all caps (NETDATA).

#### Capitalization of 'Netdata' and 'netdata'

Whenever you refer to the company Netdata, Inc., or the open-source monitoring agent the company develops, capitalize
**Netdata**.

However, if you are referring to a process, user, or group on a Linux system, you should not capitalize, as by default
those are typically lowercased. In this case, you should also fence these terms in an inline code block: `` `netdata`
``.

```markdown
# Not recommended
The netdata agent, which spawns the netdata process, is actively maintained by netdata, inc.

# Recommended
The Netdata Agent, which spawns the `netdata` process, is actively maintained by Netdata, Inc.
```

#### Capitalization of 'Agent' and 'Cloud'

Netdata is split into two products: the open source monitoring **Agent**, and the closed source web application
**Cloud**. Because both Agent and Cloud are formal nouns, you should capitalize them.

#### Capitalization of document titles and page headings

Document titles and page headings should use sentence case. That means you should only capitalize the first word.

If you need to use the name of a brand, software, product, and company, capitalize it according to their official
guidelines.

Also, don't put a period (`.`) or colon (`:`) at the end of a title or header.

```markdown
# Not recommended
Getting Started Guide
Service Discovery and Auto-Detection:
Install netdata with docker

# Recommended
Getting started guide
Service discovery and auto-detection
Install Netdata with Docker
```

### Second person

When writing documentation, you should use the second person ("you") to give instructions. When using the second person,
you give the impression that you're personally leading your reader through the steps or tips in question.

See how that works? It's a core part of making Netdata's documentation feel welcoming to all.

Avoid using "we," "I," "let's," and "us" in documentation whenever possible.

The "you" pronoun can also be implied, depending on your sentence structure. 

```markdown
# Not recommended
To install Netdata, we should try the one-line installer...

# Recommended
To install Netdata, you should try the one-line installer...

# Recommended, implied "you"
To install Netdata, try the one-line installer...
```

### Active voice

Use active voice instead of passive voice, because the active voice is more concise and easier to understand.

When using voice, the subject of the sentence is action. In passive voice, the subject is acted upon. A famous example
of passive voice is the phrase "mistakes were made."

```plain
# Not recommended (passive)
When an alarm is triggered by a metric, a notification is sent by Netdata...

# Recommended (active)
When a metric triggers an alarm, Netdata sends a notification...
```

### Standard American spelling

While the Netdata team is mostly _not_ American, we still aspire to use American spelling whenever possible, as it is
the standard for the monitoring industry.

### Clause order

If you want to instruct your reader to take some action in a particular circumstance, such as optional steps, the
beginning of the sentence should indicate that circumstance.

```markup
# Not recommended
Read the reference guide if you'd like to learn more about custom dashboards.

# Recommended
If you'd like to learn more about custom dashboards, read the reference guide.
```

By placing the circumstance at the beginning of the sentence, readers can immediately know if they want to read more or
follow a link.

### Oxford comma

The Oxford comma is the comma used after the second-to-last item in a list of three or more items. It appears just
before "and" or "or."

```markup
# Not recommended
Netdata can monitor RAM, disk I/O, MySQL queries per second and lm-sensors.

# Recommended
Netdata can monitor RAM, disk I/O, MySQL queries per second, and lm-sensors.
```

### Abbreviations (acronyms and initialisms)

Use abbreviations (including [acronyms and initialisms](https://www.dictionary.com/e/acronym-vs-abbreviation/)) in
documentation when one exists, when it's widely accepted within the monitoring/sysadmin community, and when it improves
the readability of a document.

When introducing an abbreviation to a document for the first time, give the reader both the spelled-out version and the
shortened version at the same time. For example:

```markup
You can use Netdata to monitor Extended Berkeley Packet Filter (eBPF) metrics in real-time.
```

After you define an abbreviation, don't switch back and forth—use only the abbreviation for the rest of the document.

You can also use abbreviations in a document's title to keep the title short and relevant. If you do this, you should
still introduce the spelled-out name alongside the abbreviation as soon as possible.

```markup
# Monitoring HDFS with Netdata

You can now use Netdata to collect real-time metrics from your Hadoop Distributed File System (HDFS).
```

## Markdown syntax

The Netdata documentation uses the Markdown syntax for styling and formatting. If you're not familiar with how it works,
please read the [Markdown introduction post](https://daringfireball.net/projects/markdown/) by its creator, followed by
[Mastering Markdown](https://guides.github.com/features/mastering-markdown/) guide from GitHub.

You can follow the syntax specified in the above resources for the majority of documents, but the following sections
specify a few particular use cases.

### Linking between documents

Documentation should link to relevant pages whenever it's relevant and provides valuable context to the reader. To
ensure links function properly on both GitHub and our generated documentation on [Netdata
Learn](https://learn.netdata.cloud/), links should always reference the full path to the document, beginning at the root
of the Agent repository (`/`). Links should also always end with the filename of the destination document, ending in the
`.md` extension.

Avoid relative links or traversing up directories using `../`.

For example, if you want to link to our installation guide, you should link to `/packaging/installer/README.md`. To
reference the guide for increasing metrics storage, use `/docs/guides/longer-metrics-storage.md`.

### References to UI elements

If you need to instruct your reader to click a user interface (UI) element inside of a Netdata interface, you should
reference the label text of the link/button with Markdown's (`**bold text**`) tag.

```markdown
Click on the **Sign in** button.
```

> ⚠️ Avoid using directional language to orient readers, because not every reader can use instructions like "look at the
> top-left corner" to find their way around an interface.

If you feel that you must use directional language, perhaps use an [image](#images) (with proper alt text) instead.

### Language-specific syntax highlighting in code blocks

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
    return config_get(CONFIG_SECTION_HEALTH, "stock health configuration directory", buffer);
}
```
````

And the prettified result:

```c
inline char *health_stock_config_dir(void) {
    char buffer[FILENAME_MAX + 1];
    snprintfz(buffer, FILENAME_MAX, "%s/health.d", netdata_configured_stock_config_dir);
    return config_get(CONFIG_SECTION_HEALTH, "stock health configuration directory", buffer);
}
```

Prism also supports titles and line highlighting. See the [Docusaurus
documentation](https://v2.docusaurus.io/docs/markdown-features#code-blocks) for more information.

> ⚠️ Line numbers and highlights are not compatible with GitHub's Markdown parser, and thus will only be viewable on our
> [documentation site](https://learn.netdata.cloud/). They should be used sparingly and only when necessary.

## Accessibility

Netdata's documentation should be as accessible as possible to as many people as possible. While the rules about [tone
and content](#tone-and-content) and [language and grammar](#language-and-grammar) are helpful to an extent, we also need
some additional rules to improve the reading experience for all readers.

### Images

We have a few rules around using images. Perhaps most importantly, don't use only images to convey instructions. Each
image should be accompanied by alt text and text-based instructions to ensure that every reader can access the
information in the best way for them.

#### Alt text

Provide alt text for every image you include in Netdata's documentation. It should summarize the intent and content of
the image.

In Markdown, use the standard image syntax, `![]()`, and place the alt text between the brackets `[]`. Here's an example
using our logo:

```markdown
![The Netdata logo](../../web/gui/images/netdata-logomark.svg)
```

#### Images of text

Don't use images of text, code samples, or terminal output. Instead, put that text content in a code block so that all
devices can render it clearly and screen readers can parse it.

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fdocs%2Fcontributing%2Fstyle-guide&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
