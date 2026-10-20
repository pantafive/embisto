# The Go library

```go
import (
	"context"
	"fmt"
	"time"

	"github.com/pantafive/embisto"
	"github.com/pantafive/embisto/cache"
	"github.com/pantafive/embisto/card"
)

func preview(ctx context.Context) error {
	core := embisto.New(embisto.Defaults())
	previews := cache.New(core, cache.NewMemory(64<<20, nil), cache.Options{
		TTL:        time.Hour,
		FailureTTL: time.Minute,
	})

	result, err := previews.Extract(ctx, card.Request{URL: "https://github.com"})
	if err != nil {
		code := card.CodeOf(err) // invalid_url, blocked_url, fetch_failed, timeout, too_large, not_found
		return fmt.Errorf("no preview: %s", code)
	}
	fmt.Println(result.Preview.Title, "is cached until", result.Expires)
	return nil
}
```

The core and every layer around it are a `card.Extractor`: `Extract` gives a `card.Result`, the preview with how long its origin lets it be kept, when the cache that served it lets it go, and the provider that shaped it; `Purge` forgets a URL in every layer it passes through. Layers therefore stack in any order, a cache over a cache included. Instances that share a store, such as `cache.NewRedis`, make one preview between them when it is also passed as `cache.Options.Locker`.

`Options.HomePages` is where the core remembers what each site's home page says, so that a description a site puts on every page is not taken for one page's own; it reads home pages within the preview's deadline. `Defaults()` keeps them in memory; a shared store shares them between instances, and purging a URL forgets its site.

A preview goes through a fixed pipeline:

1. Fetch the page, unless a site provider answers for the URL by itself.
2. Run the generic sources over it: oEmbed discovery, Open Graph, JSON-LD, Twitter Cards, HTML.
3. Run the site providers that match its URL.
4. Merge the facts field by field by a priority table, with site providers first. JSON-LD leads on author and publication date.
5. Run the enrichers, such as the one that measures pictures.
6. Render the preview in a format.

Each stage is an interface in the `card` package, which imports nothing else from this module. That lets you write a site provider (`card.Provider`) or an output format (`card.Renderer`) against `card` alone, then add it to the lists in `embisto.Defaults()`. Built-in providers live in `sites` and formats in `render`, whose `Defaults()` lists embisto's own; Iframely's response and failure shapes are in `iframely`. `render.LoadTemplates` makes formats of template files in any `fs.FS`; see [templates](templates.md#in-the-library).

Site providers handle the platforms where a page alone makes a poor card. The first provider in `Options.Providers` that knows a URL is the only one asked about it; `Options.Fallbacks`, such as `sites.OEmbedList`, which knows every site on the oEmbed list, are asked only about a URL no provider knows. A provider you add to `Providers` therefore wins over them wherever it is in the list. When the page fails or turns out to be a wall, the provider that knows the requested URL stands in for it. None of them needs an API key.

| Provider | Covers | Setting |
|---|---|---|
| `sites.YouTube` | Videos, shorts, live streams and playlists at every address: youtu.be, m., music., embed and youtube-nocookie.com. It embeds its own player, honouring a shared link's `t=` start time. It stands in from YouTube's oEmbed when the page is a consent wall. A video YouTube does not know is `not_found`; a private one, or one whose embedding is off, gets no player. | `NoCookie`, from `EMBISTO_YOUTUBE_NOCOOKIE` |
| `sites.X` | Posts on x.com and twitter.com, from X's oEmbed endpoint without the page, which needs JavaScript. A deleted post is `not_found`. | — |
| `sites.Reddit` | Posts, from Reddit's oEmbed endpoint; Reddit refuses most servers' requests for its pages. A deleted post is `not_found`. | — |
| `sites.TikTok` | Videos and photo posts, from TikTok's oEmbed endpoint, after following short links. It adds TikTok's own player. | — |
| `sites.Walled` | Instagram, Facebook and LinkedIn pages are read as any other when served. When the page is a login wall, or the site refuses the request (LinkedIn answers 999), the card is an honest minimal one: the URL, the site's name and icon, `limited: login_wall`. | — |
| `sites.OEmbedList` | Every site on the [oembed.com provider list](https://oembed.com/providers.json), which ships inside the binary. | — |

Any other page that turns out to be a wall also gets a minimal card marked `limited` instead of the wall's text. Three signals count:

- the request was sent on to a login page (`login_wall`);
- the request was sent on to a consent page (`consent_wall`);
- a page below the site's root says nothing but the site's name (`blocked`).

`embisto.NewFetcher` is the guarded client on its own, for a fetcher of yours to wrap.
