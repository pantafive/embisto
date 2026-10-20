# Changelog

All notable changes are listed here. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow [Semantic Versioning](https://semver.org/).

## [Unreleased]

## [0.2.0] - 2026-10-20

### Added

- `EMBISTO_MAX_CONCURRENT_PREVIEWS` (default 32) caps the previews made at once; any more get 503 `busy` with `Retry-After`. Cache hits take no slot.
- `Discovery.Timeout` limits oEmbed discovery on its own, 1 s in the defaults.
- `embisto help`, `-h` and `--help` print usage.
- Release archives carry `THIRD_PARTY_NOTICES`, the license notices of Go, every linked module and the oembed.com provider list; the image carries it with `LICENSE` in `/usr/share/doc/embisto/`.

### Changed

- **Breaking:** the purge, `DELETE /v1/preview`, is served only on the internal listener (`EMBISTO_METRICS_ADDR`, `:9090`). The API's listener answers it with 405, and its OpenAPI spec no longer lists it.
- **Breaking:** `embisto` fails on an unknown argument instead of starting the server.
- **Breaking:** `embisto sign` fails without `EMBISTO_PUBLIC_URL` instead of printing a bare path, and startup stops on an `EMBISTO_PUBLIC_URL` that is not an absolute http or https URL without a query.
- The image exposes only port 8080; the internal listener on 9090 must not be reachable from outside.
- `/healthz`, the OpenAPI document and the User-Agent report the version stamped at build time, else the module's, else `devel`, instead of a fixed 0.1.0.
- On stop, requests in flight get 24 seconds to finish instead of 10.
- A measured JPEG's width and height follow its EXIF orientation, so a photo stored sideways is reported at the size it is shown.
- The image tags `latest` and `major.minor` move only for a release, not for a pre-release such as `v0.2.0-rc.1`, which GitHub also marks as a pre-release. A release's notes are its section of this file. The main image carries the release image's OCI labels.
- `embisto pin` saves a measured picture as its width and height in `fixture.json` instead of its bytes, and a saved page keeps no attributes on `<html>`, `<head>` and `<body>`, and only `href` on `<base>`.
- The error code `invalid_options` of `/i/` is `invalid_size`, since the size is what it refuses; `card.CodeInvalidOptions` is `card.CodeInvalidSize` and `thumbnails.ErrOptions` is `thumbnails.ErrSize`.
- The OpenAPI tag of `/i/` is `Thumbnails`, no longer `Images`.
- Package `thumbs` is renamed `thumbnails`, after the feature; its `Builtin` and `NewBuiltin` are `Resizer` and `NewResizer`.
- `card.Source`'s method is `Read`, no longer `Extract`, which is `card.Extractor`'s.
- `card.Entry` is `card.CodeEntry`.
- `sites.OEmbed` is `sites.OEmbedList`, the provider for every site on the oembed.com list.
- `cache.BackendObserver` is `cache.RedisObserver`, with `RedisUp` and `RedisFailed`.
- Iframely's shapes move from `render` to the new package `iframely`: `render.Iframely` is `iframely.Renderer`, `render.IframelyOf` is `iframely.Of`, and `IframelyResponse`, `IframelyMeta`, `IframelyLink`, `IframelyMedia`, `IframelyError` and `IframelyErrorBody` drop their prefix. Their OpenAPI schema names are unchanged.

### Removed

- The `-key` flag of `scripts/iframely-compare`, since embisto takes no API key.

### Fixed

- Instagram, Facebook and LinkedIn are recognised on any subdomain, such as `de.linkedin.com` or `web.facebook.com`, so their login walls and refusals give the honest minimal card instead of an error.
- Concurrent previews of pages with several pictures whose responses carry `Cache-Control` or `Expires` no longer crash the server with a concurrent map write.
- A preview whose page arrived no longer fails with `timeout` when its oEmbed endpoint, home page, provider or a picture does not answer; the card is made from what did.
- A client that disconnects mid-preview gets 503 `canceled` instead of 500 `internal`, and the library returns the caller's context error instead of `timeout`.
- HTML entities in oEmbed titles, authors and provider names and in JSON-LD texts are decoded.
- A Content-Type with a parameter Go cannot read keeps its media type, a missing one is sniffed from the body, and `application/json+oembed` answers are read as text instead of making a bare file card.
- A byte-order mark at the start of a body is dropped, so oEmbed JSON that carries one is no longer lost.
- A listed oEmbed endpoint is also trusted without its trailing slash or with a format extension.
- Thumbnails charge a progressive JPEG's coefficients against the resizing memory limit; one without chroma subsampling near the pixel limit is now refused with 413 `too_many_pixels`.
- Thumbnails turn or mirror a JPEG as its EXIF orientation says.
- Cards list no thumbnails for AVIF, HEIC, BMP, TIFF and JPEG XL sources, whose `/i/` URLs answered 415.
- The docs' sample cards, Go snippets and OpenAPI text of `not_found` match what the code does, and the docs say that `EMBISTO_CACHE_MEMORY=0` leaves a shared cache on.

### Security

- Embed HTML from an oEmbed answer is kept only at a listed endpoint's path or beneath it at a segment boundary; a path that only starts like a listed one, or holds a dot segment or an escaped slash, is not trusted.
- Control characters and bidi controls are dropped from every card text and whitespace runs collapsed, so `format=text` and templates pass no terminal escape or forged line.
- JSON-LD blocks over 512 KiB, or past 1 MiB on one page, are skipped before parsing, so a hostile page can no longer cost over 100 MiB of memory per preview.
- moby/go-archive 0.3.3 and moby/sys/user 0.4.1, test-only dependencies, so govulncheck reports nothing the code calls.

## [0.1.0] - 2026-10-14 [YANKED]

The first release, retracted: its test data held third-party pictures.

### Added

- `GET /v1/preview` makes a card of a URL from oEmbed, Open Graph, Twitter Cards, JSON-LD and plain HTML, as JSON, plain text, a self-contained HTML card, or a template format loaded from `EMBISTO_TEMPLATES_DIR`.
- Site providers for YouTube, X, Reddit and TikTok, honest minimal cards for login walls, and every site on the oembed.com provider list.
- Iframely-compatible `/iframely` and `/oembed`.
- Thumbnails: signed, resized JPEG, PNG and WebP copies of a card's pictures, served at `/i/`.
- A cache in memory or shared through Redis-compatible servers, keeping cards as long as their origin allows, coalescing identical requests within and across instances, and purged with `DELETE /v1/preview`.
- A guarded fetcher refusing non-public addresses at connect time, with limits on size, time and memory.
- Prometheus metrics, an OpenAPI spec with a reference page, and a debug page showing how a card was made.
- The Go library behind the server: `embisto.New`, the `cache` layers, and the `card`, `sites`, `render` and `thumbs` packages.

[Unreleased]: https://github.com/pantafive/embisto/compare/v0.2.0...HEAD
[0.2.0]: https://github.com/pantafive/embisto/releases/tag/v0.2.0
