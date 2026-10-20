package api

import (
	"context"
	"net/http"
	"reflect"

	"github.com/danielgtaylor/huma/v2"

	"github.com/pantafive/embisto"
	"github.com/pantafive/embisto/card"
	"github.com/pantafive/embisto/iframely"
	"github.com/pantafive/embisto/render"
)

const (
	formatJSON    = "json"
	formatText    = "text"
	formatOEmbed  = "oembed"
	tagPreviews   = "Previews"
	tagCompatible = "Iframely-compatible"
	tagService    = "Service"
)

// responseDefault is OpenAPI's key for the responses not listed by status, the failures here.
const responseDefault = "default"

func addRoutes(api huma.API, deps Deps) {
	addServiceRoutes(api, deps)
	addPreviewRoute(api, deps)
	addCompatibleRoutes(api, deps)
	addImageRoute(api, deps)
}

func content(api huma.API, contentType string, schema reflect.Type, example any) map[string]*huma.MediaType {
	return map[string]*huma.MediaType{contentType: {Schema: api.OpenAPI().Components.Schemas.Schema(schema, true, ""), Example: example}}
}

// previewContent is the card as JSON and, when the server has the format, as text.
func previewContent(api huma.API, json, text card.Renderer) map[string]*huma.MediaType {
	media := content(api, json.ContentType(), reflect.TypeFor[card.Preview](), examplePreview)
	if text != nil {
		media[text.ContentType()] = &huma.MediaType{Schema: &huma.Schema{Type: huma.TypeString}, Example: exampleText}
	}
	return media
}

func addServiceRoutes(api huma.API, deps Deps) {
	huma.Register(api, huma.Operation{
		OperationID: "health",
		Method:      http.MethodGet,
		Path:        "/healthz",
		Summary:     "Check the server is up",
		Description: "Answers while the process runs. Meant for a liveness probe.",
		Tags:        []string{tagService},
	}, health)

	huma.Register(api, huma.Operation{
		OperationID: "readiness",
		Method:      http.MethodGet,
		Path:        "/readyz",
		Summary:     "Check the server takes new requests",
		Description: "Fails with 503 once the server is stopping: it finishes the requests it has and takes no new ones. Meant for a readiness probe.",
		Tags:        []string{tagService},
	}, deps.ready)
}

func addPreviewRoute(api huma.API, deps Deps) {
	if renderer, ok := deps.formats[formatJSON]; ok {
		huma.Register(api, huma.Operation{
			OperationID: "preview",
			Method:      http.MethodGet,
			Path:        "/v1/preview",
			Summary:     "Preview a URL",
			Description: "Fetches the page, reads oEmbed, Open Graph, JSON-LD, Twitter Cards and plain HTML from it, and merges them into one card. " +
				"Pictures whose size the page does not state are measured. `links` lists every resource found, each with its relations and size. " +
				"Previews are cached; empty fields are left out.",
			Tags: []string{tagPreviews},
			Responses: map[string]*huma.Response{
				"200": {Description: "The card, or with format=text its plain lines.", Content: previewContent(api, renderer, deps.formats[formatText])},
				responseDefault: {
					Description: "The preview failed; `code` says why: invalid_request, invalid_url or unknown_format (400), blocked_url (403), " +
						"not_found (404, or 410 when the site says the item was removed: a deleted video or post, a page answering 404), " +
						"fetch_failed or too_large (502), timeout (504), canceled (503) when the client closed the request, " +
						"busy (503, with Retry-After) when the server is making as many previews as it may, internal (500).",
					Content: content(api, "application/problem+json", reflect.TypeFor[problem](), exampleProblem),
				},
			},
		}, deps.preview)
		listFormats(api, deps)
	}
}

// addPurgeRoute serves the purge, which empties the cache for whoever asks; it belongs on the internal listener, never
// the API's.
func addPurgeRoute(api huma.API, deps Deps) {
	huma.Register(api, huma.Operation{
		OperationID:   "purge-preview",
		Method:        http.MethodDelete,
		Path:          "/v1/preview",
		DefaultStatus: http.StatusNoContent,
	}, deps.purge)
}

func addCompatibleRoutes(api huma.API, deps Deps) {
	compatibleErrors := &huma.Response{
		Description: "The preview failed, in Iframely's shape: `code` is the HTTP status. A page that answered with an error status passes it on; " +
			"400 is an invalid URL, 403 an address that is not public, 408 a page too slow.",
		Content: content(api, "application/json", reflect.TypeFor[iframely.Error](), exampleRefusal),
	}
	if renderer, ok := deps.formats[iframely.Format]; ok {
		huma.Register(api, huma.Operation{
			OperationID: "iframely",
			Method:      http.MethodGet,
			Path:        "/iframely",
			Summary:     "Preview a URL in Iframely's format",
			Description: "The same preview as `/v1/preview`, shaped as Iframely's API answers with links grouped by rel: `meta`, `links`, and the primary embed's `rel` and `html`. " +
				"See docs/iframely-compat.md in the repository for where it differs from Iframely.",
			Tags:      []string{tagCompatible},
			Responses: map[string]*huma.Response{"200": {Description: "The preview.", Content: content(api, renderer.ContentType(), reflect.TypeFor[iframely.Response](), exampleIframely)}, responseDefault: compatibleErrors},
		}, deps.compatible(iframely.Format))
	}
	if renderer, ok := deps.formats[formatOEmbed]; ok {
		huma.Register(api, huma.Operation{
			OperationID: "oembed",
			Method:      http.MethodGet,
			Path:        "/oembed",
			Summary:     "Preview a URL as oEmbed",
			Description: "The same preview as an oEmbed 1.0 answer, with the fields Iframely adds. A page with an embed is video or rich and carries `html`; " +
				"an image URL is photo; anything else is link.",
			Tags:      []string{tagCompatible},
			Responses: map[string]*huma.Response{"200": {Description: "The oEmbed answer.", Content: content(api, renderer.ContentType(), reflect.TypeFor[render.OEmbedResponse](), exampleOEmbed)}, responseDefault: compatibleErrors},
		}, deps.compatible(formatOEmbed))
	}
}

type healthOutput struct {
	Body struct {
		Status  string `json:"status" example:"ok"`
		Version string `json:"version" example:"0.1.0"`
	}
}

func health(context.Context, *struct{}) (*healthOutput, error) {
	out := &healthOutput{}
	out.Body.Status, out.Body.Version = "ok", embisto.Version
	return out, nil
}

func (deps Deps) ready(ctx context.Context, in *struct{}) (*healthOutput, error) {
	if deps.Draining != nil && deps.Draining() {
		return nil, huma.Error503ServiceUnavailable("the server is stopping")
	}
	return health(ctx, in)
}

// listFormats writes the formats the server has, its own and its templates', as the format parameter's enum in the
// spec. The handler checks the name itself, since the list is only known at startup.
func listFormats(api huma.API, deps Deps) {
	names := make([]any, 0, len(deps.Renderers))
	for _, renderer := range deps.Renderers {
		names = append(names, renderer.Name())
	}
	operation := api.OpenAPI().Paths["/v1/preview"].Get
	for _, parameter := range operation.Parameters {
		if parameter.Name == "format" && parameter.Schema != nil {
			parameter.Schema.Enum = names
			parameter.Schema.PrecomputeMessages()
		}
	}
}
