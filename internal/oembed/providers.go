package oembed

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"sync"
)

// providersJSON is https://oembed.com/providers.json, under the MIT notice in NOTICE.
//
//go:embed providers.json
var providersJSON []byte

// Providers is the oembed.com list: which endpoint answers for which URLs.
type Providers struct {
	endpoints []endpoint
	urls      []string
}

type endpoint struct {
	url     string
	schemes []*regexp.Regexp
}

// Known is the embedded provider list, parsed once.
var Known = sync.OnceValue(func() *Providers {
	providers, err := parse(providersJSON)
	if err != nil {
		panic(err) // the embedded list is part of the build
	}
	return providers
})

func parse(raw []byte) (*Providers, error) {
	var list []struct {
		Endpoints []struct {
			URL     string   `json:"url"`
			Schemes []string `json:"schemes"`
		} `json:"endpoints"`
	}
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, fmt.Errorf("decoding the oEmbed providers: %w", err)
	}
	providers := &Providers{}
	for _, provider := range list {
		for _, e := range provider.Endpoints {
			found := endpoint{url: strings.ReplaceAll(e.URL, "{format}", "json")}
			for _, scheme := range e.Schemes {
				found.schemes = append(found.schemes, pattern(scheme))
			}
			providers.endpoints = append(providers.endpoints, found)
			providers.urls = append(providers.urls, found.url)
		}
	}
	return providers, nil
}

// pattern turns a scheme such as https://*.youtube.com/watch* into a regexp. A leading *. also matches no subdomain,
// and either scheme matches both, since providers list one form of URLs served in both.
func pattern(scheme string) *regexp.Regexp {
	_, rest, found := strings.Cut(scheme, "://")
	if !found {
		rest = scheme
	}
	quoted := regexp.QuoteMeta(rest)
	if after, ok := strings.CutPrefix(quoted, `\*\.`); ok {
		quoted = `([^/]+\.)?` + after
	}
	return regexp.MustCompile(`^https?://` + strings.ReplaceAll(quoted, `\*`, `.*`) + `$`)
}

// Endpoint is the endpoint that answers for the URL; false for none.
func (p *Providers) Endpoint(target *url.URL) (string, bool) {
	raw := target.String()
	for _, e := range p.endpoints {
		for _, scheme := range e.schemes {
			if scheme.MatchString(raw) {
				return e.url, true
			}
		}
	}
	return "", false
}

// Endpoints are the listed endpoints' URLs.
func (p *Providers) Endpoints() []string {
	return p.urls
}
