// Package thumbnails serves the pictures of a preview through embisto, resized and re-encoded: sources that expire or
// refuse hotlinking stay viewable, clients never reveal their address to other sites and download no more pixels than
// they show. URLs are signed, so /i/ only serves what the server itself handed out.
package thumbnails

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

var (
	// ErrSignature is a URL whose signature does not match.
	ErrSignature = errors.New("the signature does not match")
	// ErrOptions is a URL whose size is neither a preset nor an allowed size.
	ErrOptions = errors.New("the size is neither a preset nor an allowed size")
	// ErrShortKey is a signing key too short to resist guessing.
	ErrShortKey = errors.New("the image key is too short")
	// ErrSource is a URL whose source is not an http or https URL.
	ErrSource = errors.New("the source is not an http or https URL")
)

// MinKeyLength is the shortest signing key accepted: 256 bits.
const MinKeyLength = 32

// Mode is how a picture meets its box.
type Mode string

const (
	// Fit scales the picture down to fit inside the box, keeping its proportions; a zero side does not bound.
	Fit Mode = "fit"
	// Fill scales the picture to cover the box and crops the overflow around the center.
	Fill Mode = "fill"
)

// Box is the size a picture is scaled into; it never grows.
type Box struct {
	Width  int
	Height int
	Mode   Mode
}

// Preset is a box with a name, which URLs carry instead of the size.
type Preset struct {
	Name string
	Box  Box
}

// DefaultPresets fit a list thumbnail, a card in a feed and a full-width picture.
var DefaultPresets = []Preset{
	{Name: "thumb", Box: Box{Width: 160, Height: 160, Mode: Fill}},
	{Name: "card", Box: Box{Width: 600, Mode: Fit}},
	{Name: "large", Box: Box{Width: 1200, Mode: Fit}},
}

// AllowedSides are the only sides an explicit size may name, so that the variants of one picture, and the cache, stay
// few.
var AllowedSides = []int{0, 64, 96, 128, 160, 240, 320, 480, 600, 640, 800, 960, 1200}

var (
	presetName   = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	explicitSize = regexp.MustCompile(`^([0-9]+)x([0-9]+)-(fit|fill)$`)
)

// Signer makes and checks the URLs of thumbnails: /i/<signature>/<size>/<source>, the signature an HMAC-SHA256 over the
// size and the source, the source in unpadded base64url. The URL of a picture never changes, so it can be cached for
// good.
type Signer struct {
	key     []byte
	presets []Preset
}

// NewSigner signs with a key of at least MinKeyLength bytes, and knows the presets by name.
func NewSigner(key []byte, presets []Preset) (*Signer, error) {
	if len(key) < MinKeyLength {
		return nil, fmt.Errorf("%w: it has %d bytes; it needs at least %d", ErrShortKey, len(key), MinKeyLength)
	}
	for _, preset := range presets {
		if !presetName.MatchString(preset.Name) {
			return nil, fmt.Errorf("%w: the preset name %q", ErrOptions, preset.Name)
		}
	}
	return &Signer{key: key, presets: presets}, nil
}

// Presets are the presets the signer knows.
func (s *Signer) Presets() []Preset {
	return s.presets
}

// Path is the path of a source's variant in a size: a preset's name or an explicit WxH-mode, followed by .webp for WebP.
func (s *Signer) Path(size, source string) string {
	encoded := base64.RawURLEncoding.EncodeToString([]byte(source))
	return "/i/" + s.signature(size, encoded) + "/" + size + "/" + encoded
}

// Format is the encoding a URL asks for: the size's suffix, signed with it, so that no cache mixes formats.
type Format string

const (
	// Classic is JPEG, or PNG for a picture with transparency; the size has no suffix.
	Classic Format = ""
	// WebP is lossy WebP with transparency kept; the size ends in .webp.
	WebP Format = "webp"
)

// webpSuffix ends a size that asks for WebP.
const webpSuffix = ".webp"

// Request is what a verified URL asks for.
type Request struct {
	Source *url.URL
	Box    Box
	Format Format
}

// Verify checks a URL's signature before reading anything else in it, so that a tampered URL is refused the same way
// whatever was changed, and reads the size and the source.
func (s *Signer) Verify(signature, size, encodedSource string) (Request, error) {
	given, err := base64.RawURLEncoding.DecodeString(signature)
	if err != nil || !hmac.Equal(given, s.mac(size, encodedSource)) {
		return Request{}, ErrSignature
	}
	format := Classic
	if trimmed, ok := strings.CutSuffix(size, webpSuffix); ok {
		size, format = trimmed, WebP
	}
	box, err := s.box(size)
	if err != nil {
		return Request{}, err
	}
	raw, err := base64.RawURLEncoding.DecodeString(encodedSource)
	if err != nil {
		return Request{}, fmt.Errorf("%w: %w", ErrSource, err)
	}
	source, err := url.Parse(string(raw))
	if err != nil || source.Scheme != "http" && source.Scheme != "https" || source.Host == "" {
		return Request{}, ErrSource
	}
	return Request{Source: source, Box: box, Format: format}, nil
}

// box is the size a URL names: a preset, or an explicit size whose sides are allowed.
func (s *Signer) box(size string) (Box, error) {
	if at := slices.IndexFunc(s.presets, func(p Preset) bool { return p.Name == size }); at >= 0 {
		return s.presets[at].Box, nil
	}
	match := explicitSize.FindStringSubmatch(size)
	if match == nil {
		return Box{}, fmt.Errorf("%w: %q", ErrOptions, size)
	}
	width, _ := strconv.Atoi(match[1])
	height, _ := strconv.Atoi(match[2])
	mode := Mode(match[3])
	if !slices.Contains(AllowedSides, width) || !slices.Contains(AllowedSides, height) || width == 0 && height == 0 ||
		mode == Fill && (width == 0 || height == 0) {
		return Box{}, fmt.Errorf("%w: %q", ErrOptions, size)
	}
	return Box{Width: width, Height: height, Mode: mode}, nil
}

func (s *Signer) signature(size, encodedSource string) string {
	return base64.RawURLEncoding.EncodeToString(s.mac(size, encodedSource))
}

func (s *Signer) mac(size, encodedSource string) []byte {
	mac := hmac.New(sha256.New, s.key)
	mac.Write([]byte(size + "/" + encodedSource))
	return mac.Sum(nil)
}

// ParsePresets reads presets written as name=WxH-mode, separated by commas.
func ParsePresets(raw string) ([]Preset, error) {
	var presets []Preset
	for entry := range strings.SplitSeq(raw, ",") {
		name, size, ok := strings.Cut(strings.TrimSpace(entry), "=")
		match := explicitSize.FindStringSubmatch(size)
		if !ok || match == nil || !presetName.MatchString(name) {
			return nil, fmt.Errorf("%w: the preset %q is not name=WxH-fit or name=WxH-fill", ErrOptions, entry)
		}
		width, _ := strconv.Atoi(match[1])
		height, _ := strconv.Atoi(match[2])
		presets = append(presets, Preset{Name: name, Box: Box{Width: width, Height: height, Mode: Mode(match[3])}})
	}
	return presets, nil
}
