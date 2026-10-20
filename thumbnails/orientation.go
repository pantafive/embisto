package thumbnails

import "image"

// orientation is how a stored picture is turned to be shown, as EXIF numbers it: 1 as stored, 2 to 4 mirrored or
// half-turned, 5 to 8 with its width and height swapped.
type orientation int

func (o orientation) isTurned() bool {
	return o >= 2 && o <= 8
}

func (o orientation) isSwapped() bool {
	return o >= 5 && o <= 8
}

// shown is the bounds of a stored picture as it is shown.
func (o orientation) shown(stored image.Rectangle) image.Rectangle {
	switch {
	case o.isSwapped():
		return image.Rect(0, 0, stored.Dy(), stored.Dx())
	case o.isTurned():
		return image.Rect(0, 0, stored.Dx(), stored.Dy())
	default:
		return stored
	}
}

// from is the stored pixel shown at x, y, in a stored picture width by height.
func (o orientation) from(x, y, width, height int) (int, int) {
	switch o {
	case 2:
		return width - 1 - x, y
	case 3:
		return width - 1 - x, height - 1 - y
	case 4:
		return x, height - 1 - y
	case 5:
		return y, x
	case 6:
		return y, height - 1 - x
	case 7:
		return width - 1 - y, height - 1 - x
	case 8:
		return width - 1 - y, x
	default:
		return x, y
	}
}

// storedPlan is the plan made for the picture as shown, in the stored picture's terms, so the scaling reads the
// stored pixels and only the small output is turned.
func (o orientation) storedPlan(shaped plan, stored image.Rectangle) plan {
	if !o.isTurned() {
		return shaped
	}
	ax, ay := o.from(shaped.crop.Min.X, shaped.crop.Min.Y, stored.Dx(), stored.Dy())
	bx, by := o.from(shaped.crop.Max.X-1, shaped.crop.Max.Y-1, stored.Dx(), stored.Dy())
	shaped.crop = image.Rect(min(ax, bx), min(ay, by), max(ax, bx)+1, max(ay, by)+1).Add(stored.Min)
	if o.isSwapped() {
		shaped.mid = image.Pt(shaped.mid.Y, shaped.mid.X)
		shaped.width, shaped.height = shaped.height, shaped.width
	}
	return shaped
}

// turn is a stored picture as it is shown.
func (o orientation) turn(stored *image.RGBA) *image.RGBA {
	if !o.isTurned() {
		return stored
	}
	width, height := stored.Rect.Dx(), stored.Rect.Dy()
	shown := image.NewRGBA(o.shown(stored.Rect))
	for y := range shown.Rect.Dy() {
		for x := range shown.Rect.Dx() {
			sx, sy := o.from(x, y, width, height)
			from := stored.PixOffset(stored.Rect.Min.X+sx, stored.Rect.Min.Y+sy)
			copy(shown.Pix[shown.PixOffset(x, y):], stored.Pix[from:from+rgbaBytes])
		}
	}
	return shown
}
