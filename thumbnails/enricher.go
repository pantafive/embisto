package thumbnails

import (
	"context"
	"net/url"
	"path"
	"slices"
	"strings"

	"github.com/pantafive/embisto/card"
)

// refused are the extensions of pictures /i/ refuses, so no thumbnail is handed out for them: it serves only what it
// re-encodes, and SVG and ICO are not decoded.
var refused = []string{".svg", ".svgz", ".ico"}

// Enricher gives a preview's icon and images their variants: a signed URL for every preset, at the address clients
// reach the server at. Run it after the sizes are measured, so the variants know theirs.
type Enricher struct {
	Signer  *Signer
	BaseURL string
}

func (Enricher) Name() string {
	return "thumbnails"
}

func (e Enricher) Enrich(_ context.Context, preview card.Preview, _ card.Fetcher) (card.Preview, error) {
	preview.Icon = e.varied(preview.Icon)
	preview.Image = e.varied(preview.Image)
	preview.Images = slices.Clone(preview.Images)
	for i, image := range preview.Images {
		preview.Images[i] = e.varied(image)
	}
	return preview, nil
}

// varied is the picture with a variant for each preset; a preset that comes out the same size as one before it, as
// every preset does for a small icon, is left out.
func (e Enricher) varied(image card.Image) card.Image {
	if !isResizable(image.URL) {
		return image
	}
	image.Variants = nil
	for _, preset := range e.Signer.Presets() {
		width, height := Scaled(image.Width, image.Height, preset.Box)
		isSame := func(v card.Variant) bool { return width > 0 && v.Width == width && v.Height == height }
		if slices.ContainsFunc(image.Variants, isSame) {
			continue
		}
		image.Variants = append(image.Variants, card.Variant{
			Preset: preset.Name, URL: e.BaseURL + e.Signer.Path(preset.Name, image.URL),
			WebPURL: e.BaseURL + e.Signer.Path(preset.Name+webpSuffix, image.URL), Width: width, Height: height,
		})
	}
	return image
}

func isResizable(source string) bool {
	parsed, err := url.Parse(source)
	if err != nil || parsed.Scheme != "http" && parsed.Scheme != "https" {
		return false
	}
	return !slices.Contains(refused, strings.ToLower(path.Ext(parsed.Path)))
}
