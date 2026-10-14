package api

import (
	"github.com/danielgtaylor/huma/v2"

	"github.com/pantafive/embisto"
)

const description = `embisto turns a URL into the data a client needs to draw a link preview: title, description, site,
author, publication date, icon, image with its size, video, and the provider's embed HTML.

**Formats.** ` + "`/v1/preview`" + ` answers with embisto's own card. ` + "`/iframely`" + ` and ` + "`/oembed`" + `
answer in the formats of Iframely's API, so a client of Iframely can switch by changing the address.

**Errors.** ` + "`/v1/preview`" + ` fails with an RFC 9457 problem whose ` + "`code`" + ` says why. The compatible paths
fail in Iframely's shape: ` + "`{\"error\": {\"source\", \"code\", \"message\"}}`" + `, with the HTTP status as the code.`

// spec is the OpenAPI document's frame: its description and, when known, the public address.
func spec(publicURL string) huma.Config {
	config := huma.DefaultConfig("embisto", embisto.Version)
	// A $schema link in every answer would be a field the card contract does not have.
	config.CreateHooks = nil
	// /docs is a page of its own, to pin Scalar's version.
	config.DocsPath = ""
	config.Info.Description = description
	config.Tags = []*huma.Tag{
		{Name: tagPreviews, Description: "embisto's own card."},
		{Name: tagCompatible, Description: "The formats of Iframely's API, for clients moving from it."},
		{Name: tagImages, Description: "Thumbnails: the pictures of previews, resized and served by embisto when the server has them on."},
		{Name: tagService, Description: "Probes for an orchestrator."},
	}
	config.Info.License = &huma.License{Name: "MIT", URL: "https://github.com/pantafive/embisto/blob/main/LICENSE"}
	if publicURL != "" {
		config.Servers = []*huma.Server{{URL: publicURL}}
	}
	return config
}
