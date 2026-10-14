# Changelog

All notable changes are listed here. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow [Semantic Versioning](https://semver.org/).

## [0.1.0] - unreleased

The first release.

### Added

- `GET /v1/preview` makes a card of a URL from oEmbed, Open Graph, Twitter Cards, JSON-LD and plain HTML, as JSON, plain text, a self-contained HTML card, or a template format loaded from `EMBISTO_TEMPLATES_DIR`.
- Site providers for YouTube, X, Reddit and TikTok, honest minimal cards for login walls, and every site on the oembed.com provider list.
- Iframely-compatible `/iframely` and `/oembed`.
- An image proxy serving signed, resized JPEG, PNG and WebP variants of a card's pictures.
- A cache in memory or shared through Redis-compatible servers, keeping cards as long as their origin allows, coalescing identical requests within and across instances, and purged with `DELETE /v1/preview`.
- A guarded fetcher refusing non-public addresses at connect time, with limits on size, time and memory.
- Prometheus metrics, an OpenAPI spec with a reference page, and a debug page showing how a card was made.
- The Go library behind the server: `embisto.New`, the `cache` layers, and the `card`, `sites`, `render` and `thumbs` packages.
