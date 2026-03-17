---
name: web
summary: Search the web, fetch URLs, and read tweets.
tools: [shell]
install: true
---

# Web

Three tools for getting content from the web:

- **search** -- Exa API. Default for general web search.
- **fetch** -- curl. Default choice for reading a URL.
- **tweets** -- X API v2 via curl. Use for fetching tweet data.

For JS-heavy pages that need rendering or interaction, load the `browse` skill.

## search

Search the web using the Exa API.

Get the Exa API key from the vault and pass it as the `x-api-key` header:

```
curl -s https://api.exa.ai/search \
  -H 'Content-Type: application/json' \
  -H "x-api-key: $EXA_KEY" \
  -d '{"query":"<QUERY>","type":"auto","numResults":<N>,"contents":{"text":{"maxCharacters":4000}}}' \
  | jq '[.results[] | {title,url,text}]'
```

Parameters:
- `query` -- search query. Can be a question, topic, or descriptive phrase.
- `numResults` -- number of results (1-20, default 5).
- `category` -- optional focus: `news`, `research paper`, `tweet`, `company`, `people`. Omit for general search.

When to use:
- Current events, recent news, or live data
- Verifying facts you're unsure about
- Information beyond your memory
- Someone asks you to look something up

Tips:
- Be specific. "latest iPhone release date 2026" beats "iPhone news".
- Use `category` to narrow results when relevant.
- Results include text excerpts -- often enough without fetching the URL.
- For multiple queries, run them as separate shell commands.

## fetch

Read a URL's content. Default for any URL that isn't a tweet or JS-heavy.

Quick read:

```
curl -L -s -A 'Mozilla/5.0' "<URL>" | head -c 50000
```

For cleaner output on HTML pages, strip tags:

```
curl -L -s -A 'Mozilla/5.0' "<URL>" | python3 -c "
import sys,re
h=sys.stdin.read()
for t in ('script','style','noscript'):
    h=re.sub(f'<{t}[^>]*>.*?</{t}>','',h,flags=re.S|re.I)
print(' '.join(re.sub('<[^>]+>',' ',h).split())[:20000])
"
```

Tips:
- Exa search results include text excerpts (up to 4000 chars) -- often enough without fetching.
- Use plain curl first. Add the python strip only if the HTML is too noisy.
- For JS-heavy pages (SPAs, dashboards), load the `browse` skill.

## tweets

Fetch tweet data via X API v2.

Get the X/Twitter bearer token from the vault and pass it as the Authorization header:

```
curl -s "https://api.twitter.com/2/tweets?ids=<ID>&tweet.fields=created_at,author_id,public_metrics&expansions=author_id,attachments.media_keys&user.fields=username,name&media.fields=type,url,preview_image_url" \
  -H "Authorization: Bearer $X_TOKEN" \
  | jq '{data: .data[0], author: .includes.users[0]}'
```

Extract the tweet ID from any x.com or twitter.com URL (the long numeric string in the path).

## Install

Message the user to store the Exa API key in the vault under `exa_api_key`.
