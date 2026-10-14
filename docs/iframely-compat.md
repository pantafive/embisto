# Iframely compatibility

`/iframely` and `/oembed` answer in the formats of [Iframely](https://github.com/itteco/iframely)'s API, so a client of Iframely can switch to embisto by changing the address. Both are renderers over the same card as `/v1/preview` (`render.Iframely`, `render.OEmbed`), not separate pipelines.

The formats follow Iframely's cloud API: `links` are grouped by rel, which the open-source Iframely does with `group=true`.

## How this was checked

The open-source Iframely, commit `dbce782`, ran locally in Docker without a cache and without API keys for third-party services. It was asked about the URLs in `scripts/iframely-compare/urls.txt`, and so was embisto. The comparison script reports, per URL and endpoint:

- a differing status;
- fields only Iframely gives (*missing*) and fields only embisto gives (*extra*);
- differing values of `title`, `site`, `author`, `canonical`, `medium` and `date`.

It does not compare rel values or HTML.

```sh
# in a scratch directory outside the repository
git clone https://github.com/itteco/iframely.git && cd iframely && git checkout dbce782
printf 'export default { baseAppUrl: "http://localhost:8061", port: 8061, CACHE_ENGINE: "no-cache", CACHE_TTL: 0, RESPONSE_TIMEOUT: 10000 };\n' > config.local.js
docker build -t iframely-local . && docker run --rm -p 127.0.0.1:8061:8061 iframely-local

# in this repository, with embisto on :8080
go run ./scripts/iframely-compare -iframely http://127.0.0.1:8061 -embisto http://127.0.0.1:8080
```

The script saves both answers for every URL next to its `report.md`.

## Result, 2026-10-13

| URL | /iframely | /oembed |
|---|---|---|
| https://github.com | missing links.icon[].color, links.thumbnail[].content_length, meta.theme-color; extra links.icon[].media, links.icon[].media.height, links.icon[].media.width, url | match |
| https://www.youtube.com/watch?v=dQw4w9WgXcQ | missing links.icon[].content_length, links.player[].media.scrolling, links.thumbnail[].content_length; extra url | match |
| https://vimeo.com/76979871 | missing links.player[].media.scrolling, meta.date; extra url | match |
| https://x.com/jack/status/20 | missing links.icon, links.icon[].href, links.icon[].media, links.icon[].media.height, links.icon[].media.width, links.icon[].rel, links.icon[].type; extra url; title differs | title differs |
| https://en.wikipedia.org/wiki/Go_(programming_language) | extra links.icon[].media, links.icon[].media.height, links.icon[].media.width, meta.author, meta.date, meta.site, url | extra author, provider_name |
| https://www.theguardian.com/technology/2026/oct/13/human-connection-threat-children-ai-technology | missing links.thumbnail[].content_length, meta.category, meta.keywords, meta.theme-color; extra url; author differs; medium differs | author differs |
| https://open.spotify.com/track/4cOdK2wGLETKBW3PvgPWqT | missing meta.author_url, meta.duration, meta.medium; extra url; date differs | missing author_url |
| https://soundcloud.com/forss/flickermood | missing links.icon[].content_length, meta.medium; extra links.player[].media.aspect-ratio, meta.canonical, meta.keywords, url | match |
| https://go.dev/blog/ | missing links.thumbnail[].content_length, meta.site, meta.theme-color; extra links.icon[].media, links.icon[].media.height, links.icon[].media.width, url; medium differs | missing provider_name |
| https://www.google.com/images/branding/googlelogo/2x/googlelogo_color_272x92dp.png | missing links.file[].content_length, links.image[].content_length; extra meta.canonical, url | missing content_length |
| https://www.rfc-editor.org/rfc/rfc9457.pdf | missing messages; extra meta.canonical, url | missing messages |
| https://go.dev/this-page-does-not-exist | match | match |
| https://stackoverflow.com/questions/11227809 | match | match |
| https://pkg.go.dev/net/http | extra links.icon[].media, links.icon[].media.height, links.icon[].media.width, meta.medium, url | match |
| https://www.bbc.com/news | status 403 vs 200; missing error, error.code, error.message, error.source; extra links, links.icon, links.icon[].href, links.icon[].media, links.icon[].media.height, links.icon[].media.width, links.icon[].rel, links.icon[].type, meta, meta.canonical, meta.description, meta.medium, meta.title, rel, url | status 403 vs 200; missing error, error.code, error.message, error.source; extra description, title, type, url, version |
| https://no-such-host.invalid/ | match | match |

## Known differences

Fields embisto gives and Iframely does not:

- `url` at the top of `/iframely`: the canonical URL, as Iframely's cloud API gives it.
- Sizes of icons, measured from the first bytes of each file, and `meta.canonical` for direct links to files.
- Authors and dates from JSON-LD, and `keywords` from the page's meta keywords.

Fields Iframely gives and embisto does not:

- `content_length` of files, `media.scrolling` of players, `color` of mask icons, `meta.theme-color`, `meta.category`, `meta.duration` and the `options` of a link. The last one is Iframely's switches for its own renders.
- `messages`, such as Iframely's advice about PDFs in Google Docs Viewer.
- `meta.medium` values other than `link`, `video`, `image` and `file`. Iframely also says `article`, from `og:type`, and `audio`, for music.
- Players that autoplay. Iframely adds a second link with `autoplay=1` for providers on its whitelist.
- An icon for X posts: embisto answers them from X's oEmbed endpoint without fetching the page, where the icon is.

Behaviour:

- **Which embeds lead.** Iframely puts a player at the top level (`rel`, `html`) only for providers on its whitelist. embisto puts the HTML of any provider on the oembed.com list there, such as Vimeo's.
- **Rel values.** embisto names the sources Iframely also knows (`og`, `twitter`, `oembed`) and adds `ssl`, `html5` and `responsive`. Iframely adds rels from its domain database, such as the `allow` permissions of a player and `inline` for X posts.
- **Aspect ratios** are rounded to four places.
- **Meta values can disagree.** Titles, sites, authors and dates differ where the two read different sources, or rank them differently. embisto ranks JSON-LD first for author and date, so a page whose structured data names a staff account shows that account. X titles read "jack on X", where Iframely says "jack on Twitter / X".
- **Errors.** They have Iframely's shape, `{"error": {"source", "code", "message"}}`, with `source` set to `embisto`. A page that answers with an error status passes it on. An unreachable host is 404 and a page too slow is 408. A blocked address is 403.
- **Sites that block crawlers** may answer the two differently, because each fetches with its own User-Agent. In the run above, the BBC refused Iframely with 403 and served embisto.
