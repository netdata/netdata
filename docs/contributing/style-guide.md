<!--
title: "Netdata style guide"
description: "The _Netdata style guide_ establishes editorial guidelines for all of Netdata's writings, including documentation, articles, in-product UX copy, and more."
custom_edit_url: https://github.com/netdata/netdata/edit/master/docs/contributing/style-guide.md
-->

# Netdata style guide

The _Netdata style guide_ establishes editorial guidelines for any writing produced by the Netdata team or the Netdata
community, including documentation, articles, in-product UX copy, and more. This guide should be used by both internal
Netdata teams and external contributors to any of Netdata's open-source projects.

Netdata's writing should **empower** and **educate**. You want to help people understand Netdata's value, encourage them
to learn more, and ultimately use Netdata's products to democratize monitoring in their organizations. To achieve those
goals, your writing should be:

- **Clear**. Use simple words and sentences. Use strong, direct, and active language that encourages readers to action.
- **Concise**. Provide solutions and answers as quickly as possible. Give users the information they need right now,
  along with opportunities to learn more.
- **Universal**. Think of yourself as a guide who is tasked with giving Netdata users a tour into Netdata's products,
  features, and capabilities. Write to reach the widest possible audience. 
- **Authentic**. Be honest about what Netdata can and cannot do, while focusing on our strengths. Emphasize technical
  accuracy over hard sells and marketing jargon.
- **Playful**. Be friendly, thoughtful, and engaging. Let your personality and writing style shine.

You can achieve these goals by reading and adhering to the principles outlined below.

## Tone and voice

Netdata's writing should be authentic, passionate, playful, and respectful.

### Authentic

We upfront about who we are, what we can and can’t do, and why we do what we do
Be honest and direct; take ownership of any issues & address them
Use buzzwords or jargon, talk down to you, overpromise, hard sell

### Passionate

We are excited about what we do &  want to share it with the world
Use strong, direct language; be champions, challengers  & cheerleaders; take a position
Use the passive voice; be lukewarm or wishy-washy; sound like everyone else

### Playful
We take our products, but not ourselves, seriously
Be friendly, thoughtful, & engaging; challenge the status quo
Be too casual, obscure, or snarky at someone else’s expense

### Respectful

We treat you the way you want to be treat
Give you solutions & answers; honor your trust
Impede, confuse or frustrate you; break your trust

## Accessibility

Don't make assumptions about who the reader is or their
  level of understanding, education, or experience.



Universal communication
Netdata is a global company in every sense, with employees, contributors, and users from around the world. We strive to communicate in a way that is clear and easily understood by everyone.
Accessible and responsive communication
We need to ensure that all forms of communication address the needs of our entire community and can serve the broadest possible audience. Responsive, accessible design and communication should be at the core of what we do.

Our products and content will reach a diverse audience of users who will interact with them in different ways and with different devices. Here are some considerations to keep in mind:

Would this language make sense to someone who doesn’t work here?
Could someone quickly scan this document and understand the material?
If someone can’t see the colors, images or video, is the message still clear?
Is the markup clean and structured?
Mobile devices with accessibility features are increasingly becoming core communication tools, does this work well on them?
Does this work for red/green color blind individuals?
Does this work in low light conditions? What about for users with low vision?

Basic guidelines can help make products and content accessible. Here are some ideas:

Always create an information hierarchy with key information presented first and clearly called out to improve scannability.
Avoid directional language like “sidebar on the right of the page” or “header at the top of the page” since presentation elements may adapt for devices.
Use consistent form labels and input instructions.
Use descriptive links rather than “click here” or “learn mo4re”.
Include alt text for images and image links.
Make closed captioning available for all videos. Make information in the video available in a text format.
Aim for high contrast with visual elements. 
Ensure any information contained within a graphic element is also available as plain text. This is important not just for accessibility, but also for localization.
Test any product or content on a range of different devices to ensure compatibility and no loss of information or degradation of the user experience.

Some of these guidelines were adapted from MailChimp under the Creative Commons license.

Translation and localization
Our users will likely rely on translation by third party applications to interact with our products and content, such as web browsers or live captioning. Therefore, it is important that we keep a few things in mind when developing products and content.

Keep text out of image elements if at all possible. For example, a button on a web page should be rendered using HTML/CSS rather than relying on a PNG or GIF if it contains text so that the web browser can translate it.
Be mindful that double-byte characters may take up more space than single-byte characters in design elements, subtitling and captioning.
Avoid idioms that may not be familiar to the user or that may not make sense when translated.
Avoid local, cultural or historical references that may be unfamiliar to users.
Prioritize active, direct language; avoid the passive voice.

Unbiased communication
Netdata has a company culture built on respect, and we extend that respect to our entire community. Here are some guidelines to ensure that we communicate in an unbiased way when we are referring to people:

Avoid referring to someone’s age unless it is directly relevant; likewise, avoid referring to people with age-related descriptors like “young” or “elderly”.
Avoid disability-related idioms like “lame” or “falling on deaf ears.” Don’t refer to a person’s disability unless it’s directly relevant to what you’re writing.
Don’t call groups of people “guys.” Don’t call women “girls.”
Avoid gendered terms in favor of neutral alternatives, like “server” instead of “waitress” and “businessperson” instead of “businessman.”
It’s OK to use “they” as a singular pronoun.
When writing about a person, use their communicated pronouns. When in doubt, just ask or use their name.
When in doubt, leave it out. It is better to remove it if you can’t rephrase it.

Some of these guidelines were adapted from MailChimp under the Creative Commons license.


## Language, grammar, and mechanics

## Markdown syntax

### Syntax standards (`remark-lint`)

## Glossary of Netdata terminology and preferred spelling

The following tables describe the standard spelling, capitalization, and usage of common terms found in Netdata's
writing.

### Netdata-specific terms

| Term                        | Definition                                                                                                                                                                                                                                                                                                                                                                                                            |
| :-------------------------- | :-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **claimed node**            | A node that you've proved ownership of by completing the [claiming process](/docs/cloud/get-started#claim-a-node). The claimed node will then appear in your Space and any War Rooms you added it to.                                                                                                                                                                                                                 |
| **Netdata**                 | The company behind the open-source Netdata Agent and the Netdata Cloud web application. Never use _netdata_ or _NetData_. <br /><br />In general, focus on the user's goals, actions, and solutions rather than what the company provides. For example, write _Learn more about enabling alarm notifications on your preferred platforms_ instead of _Netdata sends alarm notifications to your preferred platforms_. |
| **Netdata Agent**           | The free and open source [monitoring agent](/docs/agent) that you can install on all of your distributed systems, whether they're physical, virtual, containerized, ephemeral, and more. The Agent monitors systems running Linux, Docker, Kubernetes, macOS, FreeBSD, and more, and collects metrics from hundreds of popular services and applications.                                                             |
| **Netdata Cloud**           | The web application hosted at [https://app.netdata.cloud](https://app.netdata.cloud) that helps you monitor an entire infrastructure of distributed systems in real time. <br /><br />Never use _Cloud_ without the preceding _Netdata_ to avoid ambiguityq.                                                                                                                                                                                                                                            |
| **Netdata community**       | Contributors to any of Netdata's [open-source projects](https://learn.netdata.cloud/contribute/projects), members of the [community forum](https://community.netdata.cloud/).                                                                                                                                                                                                                                         |
| **Netdata community forum** | The Discourse-powered forum for feature requests, Netdata Cloud technical support, and conversations about Netdata's monitoring and troubleshooting products.                                                                                                                                                                                                                                                         |
| **node**                    | Used to refer to a system on which the Netdata Agent is installed. The system can be physical, virtual, in a Docker container, and more. Depending on your infrastructure, you may have one, dozens, or hundreds of nodes. Some nodes are _ephemeral_, in that they're created/destroyed automatically by a orchestrator service.                                                                                     |
| **Space**                   | The highest level container within Netdata Cloud for a user to organize their team members and nodes within their infrastructure. A Space likely represents an entire organization or a very large team. <br /><br />_Space_ is always capitalized.                                                                                                                                                                   |
| **unreachable node**        | A claimed node with a disrupted [Agent-Cloud link](/docs/agent/aclk). Unreachable could mean the node no longer exists or is experiencing network connectivity issues with Cloud.                                                                                                                                                                                                                                     |
| **visited node**            | A node which has had its Agent dashboard directly visited by a user. A list of these is maintained on a per-user basis.                                                                                                                                                                                                                                                                                               |
| **War Room**                | A smaller grouping of nodes where users can view key metrics in real-time and monitor the health of many nodes with their alarm status. War Rooms can be used to organize nodes in any way that makes sense for your infrastructure, such as by a service, purpose, physical location, and more.  <br /><br />_War Room_ is always capitalized.                                                                       |

### Other technical terms

| Term                        | Definition                                                                                                                                                                                                   |
| :-------------------------: | :----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------: |
| **filesystem**              | Use instead of _file system_.                                                                                                                                                                                |
| **preconfigured**           | The concept that many of Netdata's features come with sane defaults that users don't need to configure in order to find value.                                                                               |
| **real time**/**real-time** | Use _real time_ as a noun phrase, most often with _in_: _Netdata collects metrics in real time_. Use _real-time_ as an adjective: _Netdata collects real-time metrics from hundreds of supported collectors. |

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fdocs%2Fcontributing%2Fstyle-guide&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
