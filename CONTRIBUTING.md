# Contributing

Thanks for helping. Issues and pull requests are welcome; for anything larger than a fix, open an issue first so we can agree on the shape.

## Setting up

You need Go (the version in `go.mod`) and Docker, which the shared-cache tests use to start Valkey. [mise](https://mise.jdx.dev) installs the rest:

```sh
mise run test    # go test ./...
mise run lint    # golangci-lint with autofix
mise run serve   # the server on :8080
```

`pre-commit install` runs the linters on every commit.

Tests never touch the internet: fetches go to local `httptest` servers or replay saved fixtures. A pull request needs `go test ./...` and `golangci-lint run` to pass.

## Adding a site provider

A site provider knows one site well enough to beat what its pages say. It implements `card.Provider` in package `sites`:

```go
type Example struct{}

func (Example) Name() string { return "example" }

// Match says whether the provider knows a URL, and whether it needs the page:
// MatchedAlone answers from the URL alone (such as from the site's oEmbed endpoint),
// MatchedWithPage reads the page and the generic sources' facts first.
func (Example) Match(u *url.URL) card.Match {
	if u.Hostname() == "example.com" && strings.HasPrefix(u.Path, "/item/") {
		return card.MatchedAlone
	}
	return card.Unmatched
}

func (Example) Provide(ctx context.Context, in card.Input) (card.Facts, error) {
	// Fetch only through in.Fetcher: it refuses private addresses and keeps to the limits.
	return card.Facts{Title: "…"}, nil
}
```

Then add it to `Providers` in `Defaults()` (`options.go`). A provider that would answer for most of the web belongs in `Fallbacks` instead, so that site-specific providers win. Return an error with `card.CodeNotFound` when the site says the item is gone; any other error lets the card go on without the provider. [sites/reddit.go](sites/reddit.go) is a small example.

Test the provider in `sites/sites_test.go` with a fake `card.Fetcher`, and pin a fixture of a real page (below).

## Adding a template format

A template format needs no Go: drop `<name>.<ext>.tmpl` into the directory `EMBISTO_TEMPLATES_DIR` names. [docs/templates.md](docs/templates.md) covers the helpers and the checks a template passes at startup, and [examples/templates](examples/templates) has four examples with golden outputs. To ship a format with embisto itself, write a `card.Renderer` in package `render` and add it to `render.Defaults()`.

## Pinning fixtures

The golden test replays saved previews through the whole pipeline. To add one from a live page:

```sh
go run ./cmd/embisto pin https://example.com/some/page
```

It saves every response the preview fetched under `testdata/fixtures/<name>`, with each page trimmed to what the sources read: no scripts, no comments, no tracking markers, and JSON-LD reduced to the keys the sources use. A picture the preview measured is saved as its width and height in `fixture.json`, not as its bytes, and replays as the header of a picture of that size. It refuses to save when the trimmed responses make a different preview. When the trimmer learns to drop more, `go run ./cmd/embisto pin -retrim` trims every saved page again. A test fails if a file under `testdata` is not text, if a peek kept a body, if a saved file holds a tracking marker (a visitor's payload, a request id, a nonce, an analytics id, a logged-in user), or if a page's JSON-LD holds a key the trimmer drops, such as the comments under a video. It does not look for anything else, such as an address or a signed URL.

After a deliberate change to the output, rewrite the golden files and review the diff:

```sh
go test ./internal/golden -args -update
```

## Style

- Comments say why, not what; names say what.
- Tests are table-driven where cases repeat, with [testify](https://github.com/stretchr/testify).
- Commit messages are lowercase and imperative: `add the vimeo provider`.

## Releasing

Move the changes under a version in `CHANGELOG.md`, set `Version` in `embisto.go` to match, and push a tag such as `v0.1.0`. The release workflow then publishes static binaries for linux, darwin and windows on amd64 and arm64, and the image `ghcr.io/pantafive/embisto` tagged with the version and `latest`. `goreleaser build --snapshot --clean` tries the binaries locally.
