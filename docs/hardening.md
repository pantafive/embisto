# Hardening

embisto fetches URLs that other people choose, so the fetcher guards the network it runs in:

- **Address checks.** Connections to loopback, private, link-local (cloud metadata), CGNAT, multicast and reserved addresses are refused. The check runs on the resolved IP at connect time, for every redirect hop, so neither a redirect nor DNS rebinding gets past it. To preview an intranet anyway, list its networks in `Options.AllowedNetworks`.
- **Request limits.**
  - Only http and https are fetched.
  - Redirects are capped.
  - Bodies are capped at 5 MiB.
  - One preview, including every request it makes, has 5 seconds in total. The page may take 3 of them; oEmbed discovery and the home page 1 each, measuring pictures 2, and none of these the last tenth: one that has not answered by then is left out of the card instead of failing it.
  - What one source or provider contributes is cut at extraction: 16 pictures, 8 videos, 16 links, a title of 512 characters, other texts of 4096, URLs of 4096 bytes, embed HTML of 64 KiB.
  - A JSON-LD block over 512 KiB, or one that would take a page's blocks past 1 MiB, is skipped unread.
  - A card finished past the deadline fails with `timeout` rather than being served.
  - The in-memory caches of cards and of home pages are bounded in bytes, not in entries.
  - Proxy environment variables are ignored.
- **Embed HTML.** The embed HTML is kept only when the oEmbed answer, after any redirects, came over https as JSON from under an endpoint on the oembed.com provider list, on the host asked. A page can name any oEmbed endpoint, and a listed host may redirect elsewhere, so any other answer contributes its title and thumbnail but not its HTML.
