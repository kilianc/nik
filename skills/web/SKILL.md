---
name: web
summary: Search the web, fetch URLs, and read tweets.
tools: [shell]
---

# Web

Three tools for getting content from the web:

- **search** -- Exa API. Default for general web search.
- **fetch** -- Exa contents API (curl fallback). Default choice for reading a URL.
- **tweets** -- X API v2 via curl. Use for fetching tweet data.

For JS-heavy pages that need rendering or interaction, use the browser tooling available in your current environment instead of this skill's curl-based flow.

## search

Search the web using the Exa API.

Get the Exa API key from the vault and pass it as the `x-api-key` header:

```
curl -s https://api.exa.ai/search \
  -H 'Content-Type: application/json' \
  -H "x-api-key: $EXA_KEY" \
  -d '{"query":"<QUERY>","type":"auto","numResults":<N>,"contents":{"highlights":{"numSentences":3},"text":{"maxCharacters":4000}}}' \
  | jq '[.results[] | {title,url,highlights,text}]'
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
- `highlights` return the most relevant passages for your query -- check these first before reading full `text`.
- For multiple queries, run them as separate shell commands.

## fetch

Read a URL's content. Default for any URL that isn't a tweet or JS-heavy.

Use the Exa `/contents` endpoint for clean, LLM-ready text extraction:

```
curl -s https://api.exa.ai/contents \
  -H 'Content-Type: application/json' \
  -H "x-api-key: $EXA_KEY" \
  -d '{"urls":["<URL>"],"text":{"maxCharacters":20000}}' \
  | jq -r '.results[0].text'
```

Fallback for non-HTML content (raw text, APIs, files) or when the Exa key isn't available:

```
curl -L -s -A 'Mozilla/5.0' "<URL>" | head -c 50000
```

Tips:
- Exa handles JS-rendered pages and complex layouts natively -- no manual tag stripping needed.
- Search results already include highlights and text excerpts -- often enough without fetching.
- For JS-heavy pages that need interaction (not just reading), switch to the browser tooling available in your current environment.

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

Message the user to store these in the vault:
- `exa_api_key` -- Exa search API key
- `x_bearer_token` -- X/Twitter API v2 bearer token (needed for tweet fetching)
