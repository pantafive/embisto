package quality_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/pantafive/embisto/card"
	"github.com/pantafive/embisto/internal/quality"
)

func TestClassify(t *testing.T) {
	image := card.Image{URL: "https://example.com/a.jpg"}
	tests := []struct {
		name    string
		preview card.Preview
		err     error
		want    quality.Class
	}{
		{"title, description and picture", card.Preview{Title: "T", Description: "D", Image: image}, nil, quality.ClassFull},
		{"a picture the URL serves", card.Preview{Type: card.TypePhoto, Image: image}, nil, quality.ClassFull},
		{"title and picture", card.Preview{Title: "T", Image: image}, nil, quality.ClassPartial},
		{"title and description", card.Preview{Title: "T", Description: "D"}, nil, quality.ClassPartial},
		{"title only", card.Preview{Title: "T"}, nil, quality.ClassMinimal},
		{"a wall", card.Preview{SiteName: "S", Limited: card.LimitLoginWall}, nil, quality.ClassWall},
		{"an error", card.Preview{}, errors.New("gone"), quality.ClassError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, quality.Classify(tt.preview, tt.err))
		})
	}
}
