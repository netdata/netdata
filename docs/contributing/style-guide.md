# Netdata style guide

This in-progress style guide establishes editorial guidelines for anyone who wants to write documentation for Netdata products.

## Table of contents

- [Welcome!](#welcome)
- [Goals of the Netdata style guide](#goals-of-the-Netdata-style-guide)
- [General principles](#general-principles)
- [Tone and content](#tone-and-content)
- [Language and grammar](#language-and-grammar)
- [Markdown syntax](#markdown-syntax)
- [Accessibility](#accessibility)


## Welcome

Here at Netdata, we believe proper documentation is essential to the success of any open-source project, and we're commited to do just that. We're relying on our community to help us create new documentation, improve existing pages, and put everything we do to the test in real-world scenarios.

We're here to make developers, sysadmins, and DevOps engineers better at their jobs, after all.

If you'd like to contribute, start by reading the [guide on contributing to documentation](contributing.md) and then read the rest of this style guide.


## Goals of the Netdata style guide

An editorial style guide tries to establish a few important standards for how documentation is written and maintained, always focusing on the following principles:

- Consistency
- High-quality writing
- Conciseness
- Accessibility

By following these principles, we can make better documentation for *everyone* who wants to use Netdata, whether they're a beginner or an expert, and for everything from personal projects to mission-critical infrastructures with thousands of machines.


### Breaking the rules

None of the rules described in this style guide are absolute.

You'll come across broken rules across the existing documentation, and improvements will often require these types of deviates as well. **We welcome rule-breaking if it creates better, more accessible documentation.**

However, be aware that if you deviate too far from these established rules you may be asked to justify your decisions during the PR review process.

## General principles

This style guide is pretty overwhelming. The following list represents a few of the more essential points and rules featured in this style guide. 

 Where relevant, they link to more in-depth information about a given rule.

**[Tone and content](#tone-and-content)**:

- Be [conversational and friendly](#conversational-and-friendly-tone).
- Write [concisely](#write-concisely).
- Don't use words like **here** when [creating hyperlinks](#use-informational-hyperlinks).
- Don't mention [future releases or features](#mentioning-future-releases-or-features) in documentation.

**Language and grammar**

- t/k

**Markdown syntax**

- t/k

**Accessibility**

- t/k

---

## Tone and content

Generally speaking, we want Netdata's documentation to be conversational, concise, and highly informational without feeling too formal. 

Netdata's documentation isn't a textbook—it's a place for passing on our knowledge about health monitoring and performance troubleshooting to the next generation.

By following these principles on tone and content, we can ensure that more readers from every background and skill level can learn as much as possible about what Netdata does.

### Conversational and friendly tone

Netdata's documentation should be conversational and friendly. To borrow from Google's fantastic [developer style guide](https://developers.google.com/style/tone):

> Try to sound like a knowledgeable friend who understands what the developer wants to do.

Feel free to let some of your personality show! Documentation can be highly professional without being dry, formal, or overly instructive.

### Write concisely

You should always try to use as few words as possible to explain a particular feature, configuration, or process. Conciseness leads to more accurate and understandable writing.

### Use informational hyperlinks

Hyperlinks should clearly state its destination. Don't use words like **here** to describe where a link will take your reader.

```
# Not recommended
To install Netdata, click [here](https://docs.netdata.cloud/packaging/installer/).

# Recommended
To install Netdata, read our [installation instructions](https://docs.netdata.cloud/packaging/installer/).
```

In general, guides should include fewer hyperlinks to keep reader focused on the task at hand. Documentation should include as many hyperlinks as necessary to provide important context.

### Avoid words like "easy" or "simple"

Never assume readers of Netdata documentation are experts in Netdata's inner workings or health monitoring/performance troubleshooting in general.

If you claim that a task is easy and the reader struggles to complete it, they'll become discouraged.

If you perceive one option to be easier than another, be specific about how and why. For example, don't write, "Netdata's one-line installer is the easiest way to install Netdata." Instead you might want to say, "Netdata's one-line installer requires fewer steps than manually installing from source."

### Avoid slang, jargon, and metaphors

Netdata has a global community, and a certain phrase you're familiar with might not translate well to other cultures. Or, it may a different meaning altogether. Avoid emojis whenever possible for the same reasons.

If you must use industry jargon, such as "white-box monitoring," in a document, be sure to define the term as clearly and concisely as you can.

> White-box monitoring: Monitoring of a system or application based on the metrics it directly exposes, such as logs.

### Mentioning future releases or features

Documentation is meant to describe the product as-is, not as it will be or could be in the future. Thus Netdata documentation generally avoids talking about future features or products, even if we know they are certain.

An exception can be made for documenting beta features that are subject to change with further development.

## Language and grammar

Netdata's documentation should be consistent in the way it uses certain words, phrases, and grammar. The following sections will outline the preferred usage for capitalization, point of view, active voice, and more.

### Capitalization

In text, follow the general [English standards](https://owl.purdue.edu/owl/general_writing/mechanics/help_with_capitals.html) for capitalization. In summary:

- Capitalize the first word of every new sentence.
- Don't use uppercase for emphasis (Netdata is the BEST!).
- Capitalize the names of brands, software, products, and companies according to their official guidelines.
- Avoid camel case (NetData) or all caps (NETDATA).

#### Capitalization of 'Netdata' and 'netdata'

Whenever you refer to the company Netdata, Inc. or the open-source monitoring agent the company develops, capitalize: Netdata.

However, if you are referring to a process, user, or group on a Linux system, you should not capitalize, as by default those are typically lowercased. In this case, you should also fence these terms in an inline `` `code block` ``.

```
# Not recommended
The netdata agent, which spawns the netdata process, is actively maintained by netdata, inc.

# Recommended
The Netdata agent, which spawns the `netdata` process, is actively maintained by Netdata, Inc.
```

#### Capitalization of document titles and page headings

Document titles and page headings should use sentence case. That means you should only capitalize the first word.

If you need to use the name of a brands, software, product, and company, capitalize it according to their official guidelines.

Also, don't put a period (`.`) or colon (`:`) at the end of a title or header.

**Document titles**:

```
# Not recommended
Getting Started Guide

# Recommended
Getting started guide
```

**Page headings**:

```
# Not recommended
Service Discovery and Auto-Detection:

# Recommended
Service discovery and auto-detection
```

**Capitalization of proper nouns**:

```
# Not recommended
Install netdata with docker

# Recommended
Install Netdata with Docker
```

### Second person

When writing documentation, you should use the second person ("you") to give instructions. See how that works? When using the second person, you give the impression that you're personally leading your reader through the steps or tips in question.

See how that works? It's a core part of making Netdata's documentation feel welcoming to all.

Avoid using "we," "I", "let's," and "us" in documentation whenever possible.

The "you" pronoun can also be implied. 

```
# Not recommended
To install Netdata, we should first try the one-line installer...

# Recommended
To install Netdata, you should first try the one-line installer...

# Recommended, implied "you"
To install Netdata, first try the one-line installer...
```

### Active voice

Use active voice instead of pasive voice, because active voice is more concise and easier to understand.

In active voice, the subject of the sentence is performing the action. In passive voice, the subject is being acted upon. A famous example of passive voice is the phrase "mistakes were made."

```
# Not recommended (passive)
When an alarm is triggered by a metric, a notification is sent by Netdata...

# Recommended (active)
When a metric triggers an alarm, the system sends a notification...
```

### Standard American spelling

While the Netdata team is mostly *not* American, we still aspire to use American spelling whenever possible, as it is more commonly used within the monitoring industry.

### Clause order

If you want to instruct your reader to take some action in a particular circumstance, such as optional steps, the beginning of the sentence should incidate that circumstance.

```
# Not recommended
Read the reference guide if you'd like to learn more about custom dashboards.

# Recommended
If you'd like to learn more about custom dashboards, read the reference guide.
```

By placing the circumstance at the beginning of the sentence, you can allow people who do not want to follow the instructions stop reading and move on more quickly. And those who *do* want to read it are less likely to skip over the sentence.

### Oxford comma

The Oxford comma is the comma used after the second-to-last item in a list of three or more items. It appears just before "and" or "or."

```
# Not recommended
Netdata can monitor RAM, disk I/O, MySQL queries per second and lm-sensors.

# Recommended
Netdata can monitor RAM, disk I/O, MySQL queries per second, and lm-sensors.
```


## Markdown syntax

The Netdata documentation uses the Markdown syntax for styling and formatting. If you're not familiar with how it works, please read the [Markdown introduction post](https://daringfireball.net/projects/markdown/) by its creator, followed by [Mastering Markdown](https://guides.github.com/features/mastering-markdown/) guide from GitHub.

You can follow the syntax specified in those two posts for the majority of documents, but the following sections specify a few special use cases.

### References to UI elements

If you need to instruct your reader to click a user interface (UI) element inside of a Netdata interface, you should do two things:

1. Be as descriptive as possible about where to find the link/button, and if relevant, what the link/button looks like.
2. Reference the label text of the link/button in **bold text**.

Use Markdown's bold tag (`**text**`) to put emphasis on the UI element in question:

```markdown
Click on the **Sign in** button in the top-left of the navigation.
```

### Language-specific syntax highlighting in code blocks

Our documentation uses the [Highlight extension](https://facelessuser.github.io/pymdown-extensions/extensions/highlight/) for syntax highlighting. Highlight is fully compatible with [Pygments](http://pygments.org/), allowing you to highlight the syntax withing code blocks in a number of interesting ways.

For a full list of languages, see [Pygment's supported languages](http://pygments.org/languages/). Netdata documentation will use the following for the most part: `c`, `python`, `js`, `shell`, `markdown`, `bash`, `css`, `html`, and `go`. If no lanauge is specified, the Highlight extension not do any syntax highlighting.

Include the language directly after the three backticks (`` ``` ``) that start the code block. For highlighting C code, for example:

````
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

### Line numbers in code blocks

You can also use the Highlight and [SuperFences plugin](https://facelessuser.github.io/pymdown-extensions/extensions/superfences/) extensions together to show line numbers on the left-hand side of the code block.

To create line numbers, create one space after your syntax language declaration and then specify the starting line number with `linenums="1"`.

````
```c linenums="1"
inline char *health_stock_config_dir(void) {
    char buffer[FILENAME_MAX + 1];
    snprintfz(buffer, FILENAME_MAX, "%s/health.d", netdata_configured_stock_config_dir);
    return config_get(CONFIG_SECTION_HEALTH, "stock health configuration directory", buffer);
}
```
````

```c linenums="1"
inline char *health_stock_config_dir(void) {
    char buffer[FILENAME_MAX + 1];
    snprintfz(buffer, FILENAME_MAX, "%s/health.d", netdata_configured_stock_config_dir);
    return config_get(CONFIG_SECTION_HEALTH, "stock health configuration directory", buffer);
}
```

You can also start line numbering at a number other than 1 if you'd like to specify where the code begins in its file:

````
```c linenums="36"
inline char *health_stock_config_dir(void) {
    char buffer[FILENAME_MAX + 1];
    snprintfz(buffer, FILENAME_MAX, "%s/health.d", netdata_configured_stock_config_dir);
    return config_get(CONFIG_SECTION_HEALTH, "stock health configuration directory", buffer);
}
```
````

```c linenums="36"
inline char *health_stock_config_dir(void) {
    char buffer[FILENAME_MAX + 1];
    snprintfz(buffer, FILENAME_MAX, "%s/health.d", netdata_configured_stock_config_dir);
    return config_get(CONFIG_SECTION_HEALTH, "stock health configuration directory", buffer);
}
```

### Highlighted lines in code blocks

If you want to direct readers to a specific line within a code block, you can use the `hl_lines` option to highlight as many lines as you would like.

A single line:

````
```c hl_lines="2"
inline char *health_stock_config_dir(void) {
    char buffer[FILENAME_MAX + 1];
    snprintfz(buffer, FILENAME_MAX, "%s/health.d", netdata_configured_stock_config_dir);
    return config_get(CONFIG_SECTION_HEALTH, "stock health configuration directory", buffer);
}
```
````

```c hl_lines="2"
inline char *health_stock_config_dir(void) {
    char buffer[FILENAME_MAX + 1];
    snprintfz(buffer, FILENAME_MAX, "%s/health.d", netdata_configured_stock_config_dir);
    return config_get(CONFIG_SECTION_HEALTH, "stock health configuration directory", buffer);
}
```

Or multiple lines:

````
```c hl_lines="1 2 4"
inline char *health_stock_config_dir(void) {
    char buffer[FILENAME_MAX + 1];
    snprintfz(buffer, FILENAME_MAX, "%s/health.d", netdata_configured_stock_config_dir);
    return config_get(CONFIG_SECTION_HEALTH, "stock health configuration directory", buffer);
}
```
````

```c hl_lines="1 2 4"
inline char *health_stock_config_dir(void) {
    char buffer[FILENAME_MAX + 1];
    snprintfz(buffer, FILENAME_MAX, "%s/health.d", netdata_configured_stock_config_dir);
    return config_get(CONFIG_SECTION_HEALTH, "stock health configuration directory", buffer);
}
```

### Combining all the highlighting functions

Let's get a little wild!

```c linenums="36" hl_lines="1 2 4"
inline char *health_stock_config_dir(void) {
    char buffer[FILENAME_MAX + 1];
    snprintfz(buffer, FILENAME_MAX, "%s/health.d", netdata_configured_stock_config_dir);
    return config_get(CONFIG_SECTION_HEALTH, "stock health configuration directory", buffer);
}
```

## Accessibility

> t/k

### Images

> t/k