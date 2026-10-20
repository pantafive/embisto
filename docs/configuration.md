# Configuration

The server reads its settings from the environment; `embisto help` lists its commands, and any other argument is refused.

| Variable | Default | |
|---|---|---|
| `EMBISTO_ADDR` | `:8080` | Listen address of the API. |
| `EMBISTO_METRICS_ADDR` | `:9090` | Listen address of the internal listener: `/metrics`, the purge `DELETE /v1/preview` and `/debug`. It has no access control: keep it unreachable from outside; see [hardening](hardening.md). |
| `EMBISTO_PUBLIC_URL` | | The address clients reach the API at, an http or https URL such as `https://embisto.example.com`, with no query: listed as the server in `/openapi.json` and `/docs`, and the start of every thumbnail's URL. Anything else stops startup. |
| `EMBISTO_IMAGE_KEY` | | Signs the URLs of [thumbnails](thumbnails.md), at least 32 bytes (`openssl rand -hex 32`). Empty turns thumbnails off. |
| `EMBISTO_IMAGE_PRESETS` | `thumb=160x160-fill,card=600x0-fit,large=1200x0-fit` | The sizes of thumbnails. |
| `EMBISTO_DEBUG` | `false` | Serve `/debug` on the internal listener, never on the API's. |
| `EMBISTO_TEMPLATES_DIR` | | A directory of `<name>.<ext>.tmpl` files, each the format `<name>`; see [templates](templates.md). A template that fails its checks stops startup. |
| `EMBISTO_YOUTUBE_NOCOOKIE` | `false` | Embed YouTube's player from youtube-nocookie.com, which sets no cookies until the video plays. |
| `EMBISTO_DEFAULT_ACCEPT_LANGUAGE` | `en` | The `Accept-Language` pages are asked in. Without it, sites answer in the language of wherever the server runs. A request can name its own with `?lang=`. |
| `EMBISTO_CACHE_MEMORY` | `128MiB` | How much memory the previews kept in the process may take, such as `512MiB`; the least recently used go first. Without `EMBISTO_CACHE_URL`, `0` turns the cache off; with it, the shared cache stays on, and at least 1 MiB serves while it does not answer. |
| `EMBISTO_CACHE_MIN_TTL` | `5m` | The shortest time a preview is kept, even when its page forbids caching. |
| `EMBISTO_CACHE_MAX_TTL` | `24h` | The longest time a preview is kept, whatever its page allows. |
| `EMBISTO_CACHE_URL` | | A `redis://` or `rediss://` URL of a cache shared by every instance: Redis, Valkey, KeyDB or Dragonfly. Empty keeps previews in each instance's memory. |
| `EMBISTO_MAX_CONCURRENT_PREVIEWS` | `32` | How many previews an instance makes at once. Any more is refused with 503 `busy` and `Retry-After`; a preview served from the cache, or one a request already waits for, takes no slot. |
| `EMBISTO_LOG_LEVEL` | `info` | `debug`, `info`, `warn` or `error`. Logs are JSON. |

## Memory

What embisto keeps in memory is bounded, and adds up to some 450 MiB at the defaults:

- the previews kept in the process, `EMBISTO_CACHE_MEMORY` (128 MiB); with a shared cache, the same amount serves while it does not answer;
- what home pages say, 16 MiB;
- thumbnails, at most 256 MiB for the pictures being resized and four 10 MiB source files at once.

Previews being made come on top: each holds its page, up to 5 MiB, several times over while it reads it, and `EMBISTO_MAX_CONCURRENT_PREVIEWS` (32) bounds how many there are. Most pages are far smaller; lower the limit where large pages are common or memory is tight.

Go's garbage collector lets the heap grow to about twice what is live unless told otherwise, so give the process 768 MiB or more and set `GOMEMLIMIT` to some 80% of it, such as `GOMEMLIMIT=600MiB`.
