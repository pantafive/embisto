# Caching

A preview is kept as long as the response it was made from allows: its `Cache-Control` (`s-maxage` before `max-age`), or `Expires` against its own `Date`, less its `Age`. The response is the page's own, or a provider's API answer when the provider made the card without the page. An explicit origin lifetime is capped by `EMBISTO_CACHE_MAX_TTL`, never extended by the minimum. Responses with `no-store`, `no-cache`, `private`, or no remaining freshness are not stored; embisto does not revalidate stored source pages. Concurrent requests within an instance can still share the in-flight fetch. A page that says nothing is kept for an hour, within `EMBISTO_CACHE_MIN_TTL` and `EMBISTO_CACHE_MAX_TTL`, and a failure for a minute. Home-page metadata also respects the origin's lifetime and storage restrictions.

Every answer of `/v1/preview`, `/iframely` and `/oembed` carries `Cache-Control: public, max-age=<seconds left>`, so a cache after embisto expires the preview when embisto does. An uncached preview carries `Cache-Control: no-store`.

For cacheable previews, identical requests that arrive at once make one fetch. In one instance this always holds: 500 requests for one URL fetch its page once. With `EMBISTO_CACHE_URL`, it holds across instances too:

- The instance that first misses takes a lock in the shared cache (`SET NX PX`), makes the preview, writes it and frees the lock.
- The others wait for the value, polling the cache.
- An instance that waits longer than the lock's 15 seconds makes the preview itself rather than fail; the lock of a holder that crashed expires after the same 15 seconds.

Use a shared cache when there are several instances: they then share one cache, make one preview between them for identical requests, and purge together. While it does not answer, each instance serves from its own memory, logs a warning when that starts and ends, and counts the failures. Requests are then coalesced per instance, and a purge answers 503, since what it would drop comes back with the cache. When it answers again, each instance empties its own memory, so that nothing kept during the outage outlives a purge made since. A request whose client gave up never counts as the shared cache failing.

`DELETE /v1/preview`, on the internal listener (`EMBISTO_METRICS_ADDR`, `:9090`) and never on the API's, takes the same parameters as GET and drops what GET would serve from the cache:

- With only `url`, it drops every variant of the URL: every language and size, and with them what `/iframely` and `/oembed` serve, since every format renders the same cached preview. `format` names no variant and is ignored.
- With more parameters, it drops the one variant GET with them would serve.

It answers 204 when something was dropped, and 404 (`not_cached`) when nothing was cached. A preview being made while the purge runs may still be written after it. Without a shared cache, a purge reaches only the instance that answers it. It also forgets what embisto learned of the URL's site from its home page. The library's `Purge` does the same.
