// Package embisto turns a URL into the data for a link preview: title, description, image, icon, video and the
// provider's embed HTML.
//
// A preview goes through a fixed pipeline: fetch the page, run the generic sources over it, ask the site provider
// that knows its address, merge their facts field by field, enrich the result. [New] builds the pipeline from
// [Options], which start from [Defaults]. The [Core] it returns is a [card.Extractor], as is every layer around it,
// such as the cache in package cache, so layers stack in any order:
//
//	core := embisto.New(embisto.Defaults())
//	previews := cache.New(core, cache.NewMemory(64<<20, nil), cache.Options{TTL: time.Hour, FailureTTL: time.Minute})
//	result, err := previews.Extract(ctx, card.Request{URL: "https://github.com"})
//
// The modules of each stage are in their own packages: site providers in sites, output formats in render, with
// Iframely's shapes in iframely, and thumbnails in thumbnails. The contract they share is package card.
package embisto
