# Changelog

All notable changes are listed here. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Changed

- The error code `invalid_options` of `/i/` is `invalid_size`, since the size is what it refuses; `card.CodeInvalidOptions` is `card.CodeInvalidSize` and `thumbnails.ErrOptions` is `thumbnails.ErrSize`.
- The OpenAPI tag of `/i/` is `Thumbnails`, no longer `Images`.
- Package `thumbs` is renamed `thumbnails`, after the feature; its `Builtin` and `NewBuiltin` are `Resizer` and `NewResizer`.
- `card.Source`'s method is `Read`, no longer `Extract`, which is `card.Extractor`'s.
- `card.Entry` is `card.CodeEntry`.
- `sites.OEmbed` is `sites.OEmbedList`, the provider for every site on the oembed.com list.
- `cache.BackendObserver` is `cache.RedisObserver`, with `RedisUp` and `RedisFailed`.

## [0.1.0] - 2026-10-14

The first release.

### Added

- `GET /v1/preview` makes a card of a URL from oEmbed, Open Graph, Twitter Cards, JSON-LD and plain HTML, as JSON, plain text, a self-contained HTML card, or a template format loaded from `EMBISTO_TEMPLATES_DIR`.
- Site providers for YouTube, X, Reddit and TikTok, honest minimal cards for login walls, and every site on the oembed.com provider list.
- Iframely-compatible `/iframely` and `/oembed`.
- Thumbnails: signed, resized JPEG, PNG and WebP copies of a card's pictures, served at `/i/`.
- A cache in memory or shared through Redis-compatible servers, keeping cards as long as their origin allows, coalescing identical requests within and across instances, and purged with `DELETE /v1/preview`.
- A guarded fetcher refusing non-public addresses at connect time, with limits on size, time and memory.
- Prometheus metrics, an OpenAPI spec with a reference page, and a debug page showing how a card was made.
- The Go library behind the server: `embisto.New`, the `cache` layers, and the `card`, `sites`, `render` and `thumbs` packages.

[Unreleased]: https://github.com/pantafive/embisto/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/pantafive/embisto/releases/tag/v0.1.0
