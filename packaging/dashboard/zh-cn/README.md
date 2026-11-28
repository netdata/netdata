# Simplified Chinese dashboard preview

This directory contains a community translation for the Netdata Agent v3 dashboard. The catalog pairs English display text with Simplified Chinese. It covers navigation, chart controls, alerts, metric terminology and chart explanations. The overlay applies translations to matching DOM text and attributes as the dashboard updates. Matching depends on the English wording and, for some chart and alert descriptions, the surrounding markup.

## Try it on a dashboard

Open a v3 dashboard in Chrome. In DevTools, open **Sources → Snippets** and create a snippet containing the contents of `dictionary.js` followed by `overlay.js`. Run the snippet in the dashboard tab. The language selector appears in the lower right. Choose **English** to restore the original labels or **中文（简体）** to reapply the translation. The **?** button opens a Chinese glossary.

Reloading the page clears the snippet's effects. Running it again picks up the last language selected on that origin. Official distribution would require the dashboard or its packaging to load the reviewed assets.

## Run the browser test

With Node.js and Google Chrome installed, run these commands from this directory:

```sh
npm install --no-save --package-lock=false playwright-core@1.64.1
node test.cjs
```

The test uses a synthetic page to exercise live updates, language switching, accessibility labels, form values and rendering during page startup.
