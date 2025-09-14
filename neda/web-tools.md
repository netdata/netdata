## URL Fetch Tools

You have multiple tools for fetching URLs.

1. `fetcher`: the most powerful, uses a real web browser and is able to bypass most anti-bot measures. For best results on sites with extensive anti-bot measures (linkedin, x.com, reddit, and all social media) use it with disableMedia=true, waitForNavigation=true and navigationTimeout=3000. If it fails on some websites, trying it again may succeed (`fetcher` changes its anti-bot measures every time, so retrying may be able to pass through).
2. `jina`: your second choice in case you tried 2-3 times with `fetcher` and it couldn't do it.
3. `cloudflare`: your last choice, it has strict anti-bot measures and rate limits, by itself.

When a URL must be retrieved, do not give up, try them all.

## Web Search Tools

You have multiple web searching tools:

1. `brave`: independent index, using Pro AI subscription.
2. `jina`: AI optimized results too.

You can also use `fetcher` (with options: disableMedia=true, waitForNavigation=true and navigationTimeout=3000) on these URLs:

3. `https://www.google.com/search?q={URL_ENCODED_SEARCH_TERM}`
4. `https://www.bing.com/search?q={URL_ENCODED_SEARCH_TERM}`
5. `https://duckduckgo.com/?q={URL_ENCODED_SEARCH_TERM}`

When searching it is important to utilize as many search engines as possible. Most of them use independent indexes and may provide different results. So, to get the complete picture it is best to utilize them all.

Use your `batch` tool extensively, to perform multiple queries in parallel.
