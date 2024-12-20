# Netdata style guide

The _Netdata style guide_ establishes editorial guidelines for any writing produced by the Netdata team or the Netdata community, including documentation, articles, in-product UX copy, and more.

> **Note**
> This document is meant to be accompanied by the [Documentation Guidelines](/docs/guidelines.md). If you want to contribute to Netdata's documentation, please read it too.

Both internal Netdata teams and external contributors to any of Netdata's open-source projects should reference and adhere to this style guide as much as possible.

Netdata's writing should **empower** and **educate**. You want to help people understand Netdata's value, encourage them to learn more, and ultimately use Netdata's products to democratize monitoring in their organizations.
To achieve these goals, your writing should be:

- **Clear**. Use simple words and sentences. Use strong, direct, and active language that encourages readers to action.
- **Concise**. Provide solutions and answers as quickly as possible. Give users the information they need right now,
  along with opportunities to learn more.
- **Universal**. Think of yourself as a guide giving a tour of Netdata's products, features, and capabilities to a
  diverse group of users. Write to reach the widest possible audience.

You can achieve these goals by reading and adhering to the principles outlined below.

## Voice and tone

One way we write empowering, educational content is by using a consistent voice and an appropriate tone.

_Voice_ is like your personality, which doesn't really change day to day.

_Tone_ is how you express your personality. Your expression changes based on your attitude or mood, or based on whom
you're around. In writing, you reflect tone in your word choice, punctuation, sentence structure, or emoji.

The same idea about voice and tone applies to organizations, too. Our voice shouldn't change much between two pieces of
content, no matter who wrote each, but the tone might be quite different based on who we think is reading.

### Voice

Netdata's voice is authentic, passionate, playful, and respectful.

- **Authentic** writing is honest and fact-driven. Focus on Netdata's strength while accurately communicating what
  Netdata can and can’t do, and emphasize technical accuracy over hard sells and marketing jargon.
- **Passionate** writing is strong and direct. Be a champion for the product or feature you're writing about, and let
  your unique personality and writing style shine.
- **Playful** writing is friendly, thoughtful, and engaging. Don't take yourself too seriously, as long as it's not at
  the expense of Netdata or any of its users.
- **Respectful** writing treats people the way you want to be treated. Prioritize giving solutions and answers as
  quickly as possible.

### Tone

Netdata's tone is fun and playful, but clarity and conciseness come first. We also tend to be informal, and aren't
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
- Could anyone quickly scan this document and understand the material?
- Create an information hierarchy with key information presented first and clearly called out to improve clarity and readability.
- Avoid directional language like "sidebar on the right of the page" or "header at the top of the page" since
  presentation elements may adapt for devices.
- Use descriptive links rather than "click here" or "learn more".
- Include alt text for images and image links.
- Ensure any information contained within a graphic element is also available as plain text.
- Avoid idioms that may not be familiar to the user, or that may not make sense when translated.
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

To ensure Netdata's writing is clear, concise, and universal, we’ve established standards for language, grammar, and
certain writing mechanics. However, if you're writing about Netdata for an external publication, such as a guest blog
post, follow that publication's style guide or standards, while keeping
the [preferred spelling of Netdata terms](#netdata-specific-terms) in mind.

### Active voice

Active voice is more concise and easier to understand compared to passive voice. When using active voice, the subject of
the sentence is action. In passive voice, the subject is acted upon. A famous example of passive voice is the phrase
"mistakes were made."

|                 |                                                                                           |
|-----------------|-------------------------------------------------------------------------------------------|
| Not recommended | When an alert is triggered by a metric, a notification is sent by Netdata.                |
| **Recommended** | When a metric triggers an alert, Netdata sends a notification to your preferred endpoint. |

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
complete it, you
may inadvertently discourage them.

However, if you give users two options and want to relay that one option is genuinely less complex than another, be
specific about how and why.

For example, don't write, "Netdata's one-line installer is the easiest way to install Netdata." Instead, you might want
to say, "Netdata's one-line installer requires fewer steps than manually installing from source."

### Slang, metaphors, and jargon

A particular word, phrase, or metaphor you're familiar with might not translate well to the other cultures featured
among Netdata's global community. We recommended you avoid slang or colloquialisms in your writing.

In addition, don't use abbreviations that haven’t yet been defined in the content. See our section on
[abbreviations](#abbreviations-acronyms-and-initialisms) for additional guidance.

If you must use industry jargon, such as "mean time to resolution," define the term as clearly and concisely as you can.

> Netdata helps you reduce your organization's mean time to resolution (MTTR), which is the average time the responsible
> team requires to repair a system and resolve an ongoing incident.

### Spelling

While the Netdata team is mostly _not_ American, we still aspire to use American spelling whenever possible, as it is
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

Whenever you refer to the company Netdata Inc., or the open-source monitoring Agent the company develops, capitalize both words.

However, if you’re referring to a process, user, or group on a Linux system, use lowercase and fence the word in an
inline code block: `` `netdata` ``.

|                 |                                                                                                |
|-----------------|------------------------------------------------------------------------------------------------|
| Not recommended | The netdata agent, which spawns the netdata process, is actively maintained by Netdata Inc.    |
| **Recommended** | The Netdata Agent, which spawns the `netdata` process, is actively maintained by Netdata Inc.  |

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

After you define an abbreviation, don't switch back and forth. Use only the abbreviation for the rest of the document.

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

Do not mention future releases or upcoming features in writing unless they’ve been previously communicated via a
public roadmap.

In particular, documentation must describe, as accurately as possible, the Netdata Agent _as of the [latest
commit](https://github.com/netdata/netdata/commits/master) in the GitHub repository_. For Netdata Cloud, documentation
must reflect the _current state of [production](https://app.netdata.cloud).

### Informational links

Every link should clearly state its destination. Don't use words like "here" to describe where a link will take your
reader.

|                 |                                                                                           |
|-----------------|-------------------------------------------------------------------------------------------|
| Not recommended | To install Netdata, click [here](/packaging/installer/README.md).                         |
| **Recommended** | To install Netdata, read the [installation instructions](/packaging/installer/README.md). |

Use links as often as required to provide the necessary context. Blog posts and guides require fewer hyperlinks than
documentation.

### Contractions

Contractions like "you'll" or "they're" are acceptable in most Netdata writing. They're both authentic and playful, and
reinforce the idea that you, as a writer, are guiding users through a particular idea, process, or feature.

Contractions are generally not used in press releases or other media engagements.

### Emoji

Emoji can add fun and character to your writing, but should be used sparingly and only if it matches the content's tone
and desired audience.

## Technical/Linux standards

Configuration or maintenance of the Netdata Agent requires some system administration skills, such as navigating
directories, editing files, or starting/stopping/restarting services. Certain processes

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

For example, the [configuration](/docs/netdata-agent/configuration/README.md) doc first
teaches users how to find the Netdata config
directory and navigate to it, then runs commands from the `/etc/netdata` path so that the instructions are more
universal.

Don't include full paths, beginning from the system's root (`/`), as these might not work on certain systems.

|                 |                                                                                                                                                                                                                                                                         |
|-----------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| Not recommended | Use `edit-config` to edit Netdata's configuration: `sudo /etc/netdata/edit-config netdata.conf`.                                                                                                                                                                        |
| **Recommended** | Use `edit-config` to edit Netdata's configuration by first navigating to your [Netdata config directory](/docs/netdata-agent/configuration/README.md#the-netdata-config-directory), which is typically at `/etc/netdata`, then running `sudo edit-config netdata.conf`. |

### `sudo`

Include `sudo` before a command if you believe most Netdata users will need to elevate privileges to run it. This makes
our writing more universal, and users on `sudo`-less systems are generally already aware that they need to run commands
differently.

For example, most users need to use `sudo` with the `edit-config` script, because the Netdata config directory is owned
by the `netdata` user. The same goes for restarting the Netdata Agent with `systemctl`.

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

In Markdown, use the standard image syntax, `![]()`, and place the alt text between the brackets `[]`. Here's an example
using our logo:

```markdown
![The Netdata logo](https://github.com/netdata/netdata/blob/master/src/web/gui/static/img/netdata-logomark.svg)
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

### Adding Notes

Notes inside files should render properly both in GitHub and in Learn, to do that, it is best to use the format listed below:

```md
> **Note**
> This is an info or a note block.

> **Tip, Best Practice**
> This is a tip or a best practice block.

> **Warning, Caution**
> This is a warning or a caution block.
```

Which renders into:

> **Note**
> This is an info or a note block.

> **Tip, Best Practice**
> This is a tip or a best practice block.

> **Warning, Caution**
> This is a warning or a caution block.

### Tabs

Docusaurus allows for Tabs to be used, but we have to ensure that a user accessing the file from GitHub doesn't notice any weird artifacts while reading. So, we use tabs only when necessary in this format:

```

<Tabs>
<TabItem value="tab1" label="tab1">
  
<h3> Header for tab1 </h3>
  
text for tab1, both visible in GitHub and Docusaurus
    
    
</TabItem>
<TabItem value="tab2" label="tab2">
    
<h3> Header for tab2 </h3>
    
text for tab2, both visible in GitHub and Docusaurus

</TabItem>
</Tabs>
```

## Word list

The following tables describe the standard spelling, capitalization, and usage of words found in Netdata's writing.

### Netdata-specific terms

| Term                        | Definition                                                                                                                                                                                                                                                                                                                                                                                                            |
|-----------------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| **Connected Node**          | A node that you've proved ownership of by completing the [connecting to Cloud process](/src/claim/README.md). The claimed node will then appear in your Space and any Rooms you added it to.                                                                                                                                                                                                                          |
| **Netdata**                 | The company behind the open-source Netdata Agent and the Netdata Cloud web application. Never use _netdata_ or _NetData_. <br /><br />In general, focus on the user's goals, actions, and solutions rather than what the company provides. For example, write _Learn more about enabling alert notifications on your preferred platforms_ instead of _Netdata sends alert notifications to your preferred platforms_. |
| **Netdata Agent**           | The free and open source [monitoring agent](https://github.com/netdata/netdata) that you can install on all of your distributed systems, whether they're physical, virtual, containerized, ephemeral, and more. The Agent monitors systems running Linux, Docker, Kubernetes, macOS, FreeBSD, and more, and collects metrics from hundreds of popular services and applications.                                      |
| **Netdata Cloud**           | The web application hosted at [https://app.netdata.cloud](https://app.netdata.cloud) that helps you monitor an entire infrastructure of distributed systems in real time. <br /><br />Never use _Cloud_ without the preceding _Netdata_ to avoid ambiguity.                                                                                                                                                           |
| **Netdata community forum** | The Discourse-powered forum for feature requests, Netdata Cloud technical support, and conversations about Netdata's monitoring and troubleshooting products.                                                                                                                                                                                                                                                         |
| **Node**                    | A system on which the Netdata Agent is installed. The system can be physical, virtual, in a Docker container, and more. Depending on your infrastructure, you may have one, dozens, or hundreds of nodes. Some nodes are _ephemeral_, in that they're created/destroyed automatically by an orchestrator service.                                                                                                     |
| **Space**                   | The highest level container within Netdata Cloud for a user to organize their team members and nodes within their infrastructure. A Space likely represents an entire organization or a large team. <br /><br />_Space_ is always capitalized.                                                                                                                                                                        |
| **Unreachable node**        | A connected node with a disrupted [Agent-Cloud link](/src/aclk/README.md). Unreachable could mean the node no longer exists or is experiencing network connectivity issues with Cloud.                                                                                                                                                                                                                                |
| **Visited Node**            | A node which has had its Agent dashboard directly visited by a user. A list of these is maintained on a per-user basis.                                                                                                                                                                                                                                                                                               |
| **Room**                    | A smaller grouping of nodes where users can view key metrics in real-time and monitor the health of many nodes with their alert status. Rooms can be used to organize nodes in any way that makes sense for your infrastructure, such as by a service, purpose, physical location, and more.  <br /><br />_Room_ is always capitalized.                                                                               |

### Other technical terms

| Term                        | Definition                                                                                                                                                                                                                  |
|-----------------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| **filesystem**              | Use instead of _file system_.                                                                                                                                                                                               |
| **pre-configured**          | The concept that many of Netdata's features come with sane defaults that users don't need to configure to find immediate value.                                                                                             |
| **real time**/**real-time** | Use _real time_ as a noun phrase, most often with _in_: _Netdata collects metrics in real time_. Use _real-time_ as an adjective: _Netdata collects real-time metrics from hundreds of supported applications and services. |
