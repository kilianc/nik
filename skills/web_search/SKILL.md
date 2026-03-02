---
name: web_search
summary: >
  Search the web for current events, news, or anything beyond your
  memory. Load this when someone asks about something recent or you need
  to verify a fact.
tools: [web_search]
---

# Web Search

## web_search

Search the web for current information using the Exa API.

- `query` -- search query. Can be a question, topic, or descriptive
  phrase.
- `num_results` -- number of results to return (1-20, default 5)
- `category` -- optional focus: `news`, `research paper`, `tweet`,
  `company`, `people`. Omit for general search.

### When to use

- Someone asks about current events, recent news, or live data
- You need to verify a fact you're unsure about
- You need information beyond what's in your memory
- Someone asks you to look something up

### Tips

- Be specific in queries. "latest iPhone release date 2026" beats
  "iPhone news".
- Use `category` to narrow results when relevant (e.g. `news` for
  current events, `research paper` for academic topics).
- Results include text highlights -- often enough to answer the
  question without needing to follow URLs.
- This tool is only available when the Exa API key is configured.
