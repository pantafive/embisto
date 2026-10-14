package render

import (
	"bytes"
	"cmp"
	_ "embed"
	"fmt"
	"html/template"
	"io"
	"net/url"
	"strings"

	"github.com/pantafive/embisto/card"
)

//go:embed card.html
var cardHTML string

// cardPage is parsed with html/template: everything a page said is escaped by context, and nothing of the provider's
// embed HTML is used.
var cardPage = template.Must(template.New("card.html").Parse(cardHTML))

// cardWidth is the width the card's picture is chosen for.
const cardWidth = 600

// maxCardDescription keeps the description to a few lines.
const maxCardDescription = 300

// limitNotes say why a walled card is empty.
var limitNotes = map[card.Limit]string{
	card.LimitLoginWall:   "The site shows this only to visitors who sign in.",
	card.LimitConsentWall: "The site asks for consent before it shows this.",
	card.LimitBlocked:     "The site did not show this to embisto.",
}

// HTML writes the preview as a self-contained HTML card, for an iframe or an email: inline CSS that follows the
// reader's light or dark preference, no JavaScript, the picture's box sized before it loads, and the WebP and
// JPEG thumbnails when the server has them on. It links to the page in a new tab, without a referrer.
type HTML struct{}

func (HTML) Name() string {
	return formatHTML
}

func (HTML) ContentType() string {
	return contentTypes[formatHTML]
}

func (HTML) Render(w io.Writer, preview card.Preview) error {
	var out bytes.Buffer
	if err := cardPage.Execute(&out, cardOf(preview)); err != nil {
		return fmt.Errorf("rendering the HTML card: %w", err)
	}
	if _, err := w.Write(out.Bytes()); err != nil {
		return fmt.Errorf("writing the HTML card: %w", err)
	}
	return nil
}

// htmlCard is what the HTML card shows.
type htmlCard struct {
	Link        string
	Title       string
	Site        string
	Description string
	Note        string
	Icon        string
	Picture     *htmlPicture
}

type htmlPicture struct {
	URL           string
	WebP          string
	Width, Height int
}

func cardOf(preview card.Preview) htmlCard {
	link := cmp.Or(preview.CanonicalURL, preview.URL)
	site := cmp.Or(preview.SiteName, hostOf(link))
	shown := htmlCard{
		Link: link, Site: site, Title: cmp.Or(preview.Title, site), Icon: preview.Icon.URL,
		Description: truncate(maxCardDescription, preview.Description), Note: limitNotes[preview.Limited],
	}
	if preview.Image.URL != "" {
		chosen := pictureFor(preview, cardWidth, false)
		webp := pictureFor(preview, cardWidth, true)
		shown.Picture = &htmlPicture{URL: chosen.URL, Width: chosen.Width, Height: chosen.Height}
		if webp.Type == "image/webp" {
			shown.Picture.WebP = webp.URL
		}
	}
	return shown
}

func hostOf(link string) string {
	parsed, err := url.Parse(link)
	if err != nil {
		return link
	}
	return strings.TrimPrefix(parsed.Hostname(), "www.")
}
