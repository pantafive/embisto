package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pantafive/embisto"
	"github.com/pantafive/embisto/card"
	"github.com/pantafive/embisto/iframely"
	"github.com/pantafive/embisto/internal/api"
)

func catalogueCodes() []string {
	codes := make([]string, 0, len(card.Catalogue()))
	for _, entry := range card.Catalogue() {
		codes = append(codes, string(entry.Code))
	}
	return codes
}

// TestSpecListsExactlyTheCatalogue fails when the served spec's codes drift from the catalogue.
func TestSpecListsExactlyTheCatalogue(t *testing.T) {
	server := httptest.NewServer(api.NewServer(api.Deps{Previews: embisto.New(embisto.Defaults()), Renderers: renderers}))
	t.Cleanup(server.Close)

	resp := get(t, server.URL+"/openapi.json")

	var spec struct {
		Components struct {
			Schemas map[string]struct {
				Properties map[string]struct {
					Enum []string `json:"enum"`
				} `json:"properties"`
			} `json:"schemas"`
		} `json:"components"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&spec))
	problem, ok := spec.Components.Schemas["Problem"]
	require.True(t, ok, "the spec has no Problem schema")
	assert.Equal(t, catalogueCodes(), problem.Properties["code"].Enum)
}

func TestHumasOwnRefusalsCarryACatalogueCode(t *testing.T) {
	server := httptest.NewServer(api.NewServer(api.Deps{Previews: embisto.New(embisto.Defaults()), Renderers: renderers}))
	t.Cleanup(server.Close)
	tests := []struct {
		name, query string
		code        card.Code
	}{
		{"a negative size", "?url=example.com&max_width=-1", card.CodeInvalidRequest},
		{"a malformed language", "?url=example.com&lang=en_US", card.CodeInvalidRequest},
		{"a format the server lacks", "?url=example.com&format=nope", card.CodeUnknownFormat},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := get(t, server.URL+"/v1/preview"+tt.query)

			var refused struct {
				Code card.Code `json:"code"`
			}
			require.NoError(t, json.NewDecoder(resp.Body).Decode(&refused))
			assert.Equal(t, []any{tt.code.Status(), tt.code}, []any{resp.StatusCode, refused.Code})
		})
	}
}

func TestCompatiblePathsRefuseMalformedParametersInIframelysShape(t *testing.T) {
	server := httptest.NewServer(api.NewServer(api.Deps{Previews: embisto.New(embisto.Defaults()), Renderers: renderers}))
	t.Cleanup(server.Close)
	for _, query := range []string{"maxwidth=-1", "maxheight=wide", "lang=en_US"} {
		t.Run(query, func(t *testing.T) {
			resp := get(t, server.URL+"/iframely?url="+url.QueryEscape("https://example.com")+"&"+query)

			var refused iframely.Error
			require.NoError(t, json.NewDecoder(resp.Body).Decode(&refused))
			assert.Equal(t, []any{http.StatusBadRequest, "application/json", http.StatusBadRequest, card.CodeInvalidRequest.Text()},
				[]any{resp.StatusCode, resp.Header.Get("Content-Type"), refused.Error.Code, refused.Error.Message})
		})
	}
}
