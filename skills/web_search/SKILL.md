---
name: web_search
summary: Search the live web for current events, news, or facts beyond your memory.
tools: [shell]
---

# Web Search

Search the web using the Exa API via `curl`.

## API key

The Exa API key is stored in `config.yaml` (home directory) under
`exa_api_key`. Read it before making any request and pass it as the
`x-api-key` header.

## Search

```
shell action: "run", command: "curl -s https://api.exa.ai/search -H 'Content-Type: application/json' -H 'x-api-key: <KEY>' -d '{\"query\":\"<QUERY>\",\"type\":\"auto\",\"numResults\":<N>,\"category\":\"<CATEGORY>\",\"contents\":{\"text\":{\"maxCharacters\":4000}}}' | jq '[.results[] | {title,url,text}]'"
```

Parameters:

- `query` -- search query. Can be a question, topic, or descriptive phrase.
- `numResults` -- number of results (1-20, default 5).
- `category` -- optional focus: `news`, `research paper`, `tweet`, `company`, `people`. Omit the field entirely for general search.

The `jq` filter extracts only `title`, `url`, and `text` from each result.

## When to use

- Someone asks about current events, recent news, or live data
- You need to verify a fact you're unsure about
- You need information beyond what's in your memory
- Someone asks you to look something up

## Tips

- Be specific in queries. "latest iPhone release date 2026" beats "iPhone news".
- Use `category` to narrow results when relevant (e.g. `news` for current events, `research paper` for academic topics).
- Results include text excerpts -- often enough to answer the question without following URLs.
- For multiple queries, run them as separate `shell` commands.
