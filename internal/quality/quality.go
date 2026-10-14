// Package quality classes a card by how good it came out, from the client's side.
package quality

import "github.com/pantafive/embisto/card"

// Class is how good a card came out, from the client's side.
type Class string

// The classes of a card, best first.
const (
	// ClassFull has a title, a description and a picture or player; or is a picture or video the URL serves itself.
	ClassFull Class = "full"
	// ClassPartial has a title and one of a description and a picture or player.
	ClassPartial Class = "partial"
	// ClassMinimal has less: often just a title.
	ClassMinimal Class = "minimal"
	// ClassWall is a card the site walled off: a login or consent page, or a refusal.
	ClassWall Class = "wall"
	// ClassError is no card at all.
	ClassError Class = "error"
)

// SourceGeneric names a card no site provider shaped, in metrics.
const SourceGeneric = "generic"

// Classify is how good a card is; an error is no card.
func Classify(preview card.Preview, err error) Class {
	switch {
	case err != nil:
		return ClassError
	case preview.Limited != "":
		return ClassWall
	default:
		return contentClass(preview)
	}
}

func contentClass(preview card.Preview) Class {
	hasPicture, isMedia := hasPicture(preview), isMedia(preview)
	hasTitle, hasDescription := preview.Title != "", preview.Description != ""
	switch {
	case isMedia && !hasTitle && hasPicture, hasTitle && hasDescription && hasPicture:
		return ClassFull
	case hasTitle && (hasDescription || hasPicture):
		return ClassPartial
	default:
		return ClassMinimal
	}
}

func hasPicture(preview card.Preview) bool {
	return preview.Image.URL != "" || preview.Video.URL != "" || preview.Embed.HTML != ""
}

func isMedia(preview card.Preview) bool {
	return preview.Type == card.TypePhoto || preview.Type == card.TypeVideo
}
