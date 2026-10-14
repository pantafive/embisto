package meta

import (
	"cmp"
	"context"
	"encoding/json"
	"slices"
	"strconv"
	"strings"

	"github.com/pantafive/embisto/card"
)

// The schema.org types JSON-LD is read for: the thing the page is about, and the organization behind it.
var (
	mainTypes = []string{
		"Article", "NewsArticle", "BlogPosting", "Report", "TechArticle", "ScholarlyArticle", "SocialMediaPosting",
		"LiveBlogPosting", "VideoObject", "Product", "Recipe", "Event", "Book", "Movie",
	}
	organizationTypes = []string{"Organization", "NewsMediaOrganization", "Corporation", "WebSite"}
)

// item is one JSON-LD node: values are strings, numbers, nodes or lists of them.
type item map[string]any

// JSONLD reads schema.org structured data. Malformed blocks are common and skipped, never failing the source.
type JSONLD struct{}

func (JSONLD) Extract(_ context.Context, in card.Input) (card.Facts, error) {
	items := make([]item, 0, len(in.Page.LinkedData))
	for _, block := range in.Page.LinkedData {
		items = append(items, nodes(block)...)
	}
	facts := card.Facts{Source: card.SourceJSONLD}
	if organization, ok := find(items, organizationTypes); ok {
		facts.SiteName = text(organization["name"])
	}
	main, ok := find(items, mainTypes)
	if !ok {
		return facts, nil
	}
	facts.Title = cmp.Or(text(main["headline"]), text(main["name"]))
	facts.Description = text(main["description"])
	facts.Author = names(main["author"])
	facts.AuthorURL = in.Page.Resolve(authorURL(main["author"]))
	facts.Keywords = keywords(main["keywords"])
	facts.PublishedAt = parseTime(cmp.Or(text(main["datePublished"]), text(main["uploadDate"])))
	facts.SiteName = cmp.Or(names(main["publisher"]), facts.SiteName)
	facts.Images = images(in.Page, main["image"], main["thumbnailUrl"])
	if slices.Contains(types(main), "VideoObject") {
		facts.Type = card.TypeVideo
		facts.Videos = videos(in.Page, main)
	}
	return facts, nil
}

// nodes are the items of a block: one object, a list of them, or a @graph.
func nodes(block string) []item {
	var parsed any
	if err := json.Unmarshal([]byte(block), &parsed); err != nil {
		return nil
	}
	var found []item
	var walk func(value any)
	walk = func(value any) {
		switch v := value.(type) {
		case []any:
			for _, each := range v {
				walk(each)
			}
		case map[string]any:
			found = append(found, v)
			walk(v["@graph"])
		}
	}
	walk(parsed)
	return found
}

func find(items []item, wanted []string) (item, bool) {
	for _, it := range items {
		for _, t := range types(it) {
			if slices.Contains(wanted, t) {
				return it, true
			}
		}
	}
	return nil, false
}

// types are an item's @type, a name or a list of names, without a schema.org prefix.
func types(it item) []string {
	var found []string
	for _, value := range list(it["@type"]) {
		if name, ok := value.(string); ok {
			found = append(found, strings.TrimPrefix(strings.TrimPrefix(name, "http://schema.org/"), "https://schema.org/"))
		}
	}
	return found
}

func list(value any) []any {
	if values, ok := value.([]any); ok {
		return values
	}
	if value == nil {
		return nil
	}
	return []any{value}
}

func text(value any) string {
	if s, ok := value.(string); ok {
		return clean(s)
	}
	return ""
}

// names are the names of people or organizations given as text, nodes or a list of either.
func names(value any) string {
	var found []string
	for _, each := range list(value) {
		name := text(each)
		if node, ok := each.(map[string]any); ok {
			name = text(node["name"])
		}
		if name != "" && !slices.Contains(found, name) {
			found = append(found, name)
		}
	}
	return strings.Join(found, ", ")
}

// authorURL is the first author's page, when the author is a node that has one.
func authorURL(value any) string {
	for _, each := range list(value) {
		if node, ok := each.(map[string]any); ok {
			return text(node["url"])
		}
	}
	return ""
}

// keywords are given as one comma-separated text or as a list.
func keywords(value any) string {
	var found []string
	for _, each := range list(value) {
		if keyword := text(each); keyword != "" {
			found = append(found, keyword)
		}
	}
	return strings.Join(found, ", ")
}

// images are pictures given as URLs, ImageObjects or lists of either.
func images(page card.Page, values ...any) []card.Image {
	var found []card.Image
	for _, value := range values {
		for _, each := range list(value) {
			image := card.Image{URL: page.Resolve(text(each))}
			if node, ok := each.(map[string]any); ok {
				image = card.Image{URL: page.Resolve(cmp.Or(text(node["url"]), text(node["contentUrl"]))), Width: dimension(node["width"]), Height: dimension(node["height"])}
			}
			if image.URL != "" {
				found = append(found, image)
			}
		}
	}
	return found
}

func videos(page card.Page, video item) []card.Video {
	width, height := dimension(video["width"]), dimension(video["height"])
	var found []card.Video
	if player := page.Resolve(text(video["embedUrl"])); player != "" {
		found = append(found, card.Video{URL: player, Type: "text/html", Width: width, Height: height})
	}
	if file := page.Resolve(text(video["contentUrl"])); file != "" {
		found = append(found, card.Video{URL: file, Type: text(video["encodingFormat"]), Width: width, Height: height})
	}
	return found
}

// dimension reads a size given as a number, a string or a QuantitativeValue.
func dimension(value any) int {
	switch v := value.(type) {
	case float64:
		return max(0, int(v))
	case string:
		n, err := strconv.Atoi(strings.TrimSuffix(strings.TrimSpace(v), "px"))
		if err != nil {
			return 0
		}
		return max(0, n)
	case map[string]any:
		return dimension(v["value"])
	default:
		return 0
	}
}
