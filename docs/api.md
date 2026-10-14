# HTTP API

The server answers on `EMBISTO_ADDR` (`:8080`); its OpenAPI spec is at `/openapi.json`, and `/docs` renders it with [Scalar](https://github.com/scalar/scalar), at a pinned version loaded from jsDelivr.

## The card

`GET /v1/preview?url=https://www.youtube.com/watch?v=dQw4w9WgXcQ` answers:

```json
{
  "url": "https://www.youtube.com/watch?v=dQw4w9WgXcQ",
  "canonical_url": "https://www.youtube.com/watch?v=dQw4w9WgXcQ",
  "type": "video",
  "title": "Rick Astley - Never Gonna Give You Up (Official Video) (4K Remaster)",
  "site_name": "YouTube",
  "author": "Rick Astley",
  "image": {"url": "https://i.ytimg.com/vi/dQw4w9WgXcQ/maxresdefault.jpg", "width": 1280, "height": 720},
  "embed": {"html": "<iframe …></iframe>", "width": 200, "height": 113, "provider": "YouTube"},
  "links": [
    {"href": "https://i.ytimg.com/vi/dQw4w9WgXcQ/maxresdefault.jpg", "rel": ["thumbnail", "opengraph", "twitter"],
     "type": "image/jpeg", "media": {"width": 1280, "height": 720, "aspect_ratio": 1.7778}},
    {"href": "https://www.youtube.com/embed/dQw4w9WgXcQ", "rel": ["player", "opengraph", "twitter"], "type": "text/html", "…": "…"}
  ],
  "sources": ["oembed", "opengraph", "jsonld", "twitter", "html"]
}
```

Empty fields are left out. `icon`, `image` and each of `images` are pictures with the fields listed under `image`.

| Field | Type | Meaning |
|---|---|---|
| `url` | string | The URL asked about. |
| `canonical_url` | string | The page's own address for itself, from its canonical link or Open Graph; else the URL that served it after redirects. |
| `type` | string | What the card shows: `link`, `video`, `photo` or `rich`. |
| `title` | string | The item's title. |
| `description` | string | The item's summary. |
| `site_name` | string | The site's name, such as YouTube. |
| `author` | string | Who made the item. |
| `author_url` | string | The author's page. |
| `keywords` | string | The page's keywords, separated by commas. |
| `published_at` | string | When the item was published, as RFC 3339. |
| `icon` | picture | The site's icon. |
| `image` | picture | The card's picture: the first of `images`. |
| `images` | array of pictures | Every picture of the item the sources found, by their priority. |
| `image.url` | string | The picture's URL on its site. |
| `image.width`, `image.height` | integer | Its size in pixels, as the page states it or measured from the file's first bytes (PNG, JPEG, GIF, WebP, SVG, ICO). |
| `image.variants` | array | The picture's thumbnails, when the server has them on; see [thumbnails](thumbnails.md). |
| `image.variants[].preset` | string | The preset's name, such as `thumb`, `card` or `large`. |
| `image.variants[].url` | string | The thumbnail as JPEG, or PNG when it has transparency. |
| `image.variants[].webp_url` | string | The thumbnail as WebP, about half the bytes. |
| `image.variants[].width`, `image.variants[].height` | integer | The thumbnail's size, when the original's is known. |
| `video` | object | What the item plays. |
| `video.url` | string | A video file, or a player page. |
| `video.type` | string | Its media type; `text/html` for a player page. |
| `video.width`, `video.height` | integer | Its size in pixels. |
| `embed` | object | The provider's own way of showing the item inline. |
| `embed.html` | string | The HTML to insert. |
| `embed.width`, `embed.height` | integer | The size the HTML asks for. |
| `embed.provider` | string | Who offers it. |
| `duration` | integer | How long the video or audio lasts, in seconds. |
| `limited` | string | Why the card has little more than its URL: `login_wall`, `consent_wall` or `blocked`. Such a card carries the site's name and icon, never the wall's text. |
| `links` | array | Every resource the sources found, each URL once. |
| `links[].href` | string | The resource's URL. |
| `links[].rel` | array of strings | What the resource is (`icon`, `thumbnail`, `image`, `player`, `app`, `file`) and every source that found it. |
| `links[].type` | string | Its media type. |
| `links[].media` | object | Its size, when known. |
| `links[].media.width`, `links[].media.height` | integer | The size in pixels. |
| `links[].media.aspect_ratio` | number | Width over height, to four places. |
| `links[].html` | string | The HTML that embeds a player or an app. |
| `sources` | array of strings | The sources the fields came from, highest priority first. |

## Routes

| Route | |
|---|---|
| `GET /v1/preview?url=` | The preview card. A URL without a scheme is taken as https. `format=text` gives plain lines instead: the title with its site, the description trimmed, and the URL. `format=html` gives a self-contained HTML card for an iframe or an email: inline CSS following light and dark, no JavaScript, and none of the provider's embed HTML. Templates add more formats; see [templates](templates.md). |
| `DELETE /v1/preview?url=` | Drops the URL's cached previews; see [caching](caching.md). |
| `GET /iframely?url=` | The same preview in the format of Iframely's API, with links grouped by rel. |
| `GET /oembed?url=` | The same preview as an oEmbed answer, with the fields Iframely adds. |
| `GET /healthz` | The process is up. |
| `GET /readyz` | The server takes new requests. It returns 503 while the server shuts down. |
| `GET /i/<signature>/<size>/<source>` | A thumbnail of a card's picture. The URLs come signed in the card's `variants`; see [thumbnails](thumbnails.md). |
| `GET :9090/metrics` | Prometheus metrics, on a separate listener. |
| `GET :9090/debug?url=` | With `EMBISTO_DEBUG=true`: how a preview is made, on the internal listener only. |

embisto has no API keys and no request limits of its own: whatever serves it to clients decides who may call it and how often. It protects itself from the pages it fetches; see [hardening](hardening.md).

`/iframely` and `/oembed` let a client of Iframely switch by changing the address. They fail in Iframely's shape rather than as problem+json. [Iframely compatibility](iframely-compat.md) lists where they differ from a local Iframely on a fixed set of URLs.

## Errors

A failed request returns a non-2xx status with an [RFC 9457](https://www.rfc-editor.org/rfc/rfc9457) `application/problem+json` body. Its `code` field tells the failures apart:

| `code` | Status | Meaning |
|---|---|---|
| `invalid_request` | 400 | A parameter is malformed or out of range, such as a negative size or a malformed language. |
| `invalid_url` | 400 | The URL is missing, malformed, or not http/https. |
| `blocked_url` | 403 | The URL resolves to a loopback, private, link-local or otherwise non-public address. |
| `fetch_failed` | 502 | The page could not be fetched: the connection was refused, the status was an error, or there were too many redirects. |
| `timeout` | 504 | The page did not answer in time. |
| `too_large` | 502 | The page is larger than the body limit. |
| `not_found` | 404 or 410 | The item does not exist or was removed: the page answered 404 or 410, or a provider says the video or post is gone. |
| `unknown_format` | 400 | The server has no such `format`. |
| `not_cached` | 404 | A purge found nothing of the URL cached. |
| `cache_unavailable` | 503 | A purge the shared cache did not answer. |
| `internal` | 500 | embisto itself failed; the cause is logged, never answered. |

The codes of `/i/` are in [thumbnails](thumbnails.md). `card.Catalogue()` lists every code with its status and the words a client is shown; the `code` enum in `/openapi.json` is generated from it. A `card.Error`'s text is always the catalogue's words or a detail written for clients: what went wrong underneath, which may name a resolved address or quote a page, is only logged.
