# Configuration

The server reads its settings from the environment.

| Variable | Default | |
|---|---|---|
| `EMBISTO_ADDR` | `:8080` | Listen address of the API. |
| `EMBISTO_METRICS_ADDR` | `:9090` | Listen address of `/metrics`. |
| `EMBISTO_PUBLIC_URL` | | The address clients reach the API at, listed as the server in `/openapi.json` and `/docs`. |
| `EMBISTO_IMAGE_KEY` | | Signs the URLs of [image variants](images.md), at least 32 bytes (`openssl rand -hex 32`). Empty turns variants off. |
| `EMBISTO_IMAGE_PRESETS` | `thumb=160x160-fill,card=600x0-fit,large=1200x0-fit` | The sizes of image variants. |
| `EMBISTO_DEBUG` | `false` | Serve `/debug` on the internal listener, never on the API's. |
| `EMBISTO_TEMPLATES_DIR` | | A directory of `<name>.<ext>.tmpl` files, each the format `<name>`; see [templates](templates.md). A template that fails its checks stops startup. |
| `EMBISTO_YOUTUBE_NOCOOKIE` | `false` | Embed YouTube's player from youtube-nocookie.com, which sets no cookies until the video plays. |
| `EMBISTO_DEFAULT_ACCEPT_LANGUAGE` | `en` | The `Accept-Language` pages are asked in. Without it, sites answer in the language of wherever the server runs. A request can name its own with `?lang=`. |
| `EMBISTO_CACHE_MEMORY` | `128MiB` | How much memory the previews kept in the process may take, such as `512MiB`; the least recently used go first. `0` turns the cache off. |
| `EMBISTO_CACHE_MIN_TTL` | `5m` | The shortest time a preview is kept, even when its page forbids caching. |
| `EMBISTO_CACHE_MAX_TTL` | `24h` | The longest time a preview is kept, whatever its page allows. |
| `EMBISTO_CACHE_URL` | | A `redis://` or `rediss://` URL of a cache shared by every instance: Redis, Valkey, KeyDB or Dragonfly. Empty keeps previews in each instance's memory. |
| `EMBISTO_LOG_LEVEL` | `info` | `debug`, `info`, `warn` or `error`. Logs are JSON. |

## Memory

What embisto keeps in memory is bounded, and adds up to some 450 MiB at the defaults:

- the previews kept in the process, `EMBISTO_CACHE_MEMORY` (128 MiB); with a shared cache, the same amount serves while it does not answer;
- what home pages say, 16 MiB;
- image variants, at most 256 MiB for the pictures being resized and four 10 MiB source files at once.

Go's garbage collector lets the heap grow to about twice what is live unless told otherwise, so give the process 768 MiB or more and set `GOMEMLIMIT` to some 80% of it, such as `GOMEMLIMIT=600MiB`.
