package card_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/pantafive/embisto/card"
)

// TestCodeOf: no error once read as fetch_failed, so a test of a failure passed on a success.
func TestCodeOf(t *testing.T) {
	assert.Equal(t, []card.Code{"", card.CodeTimeout, card.CodeFetchFailed},
		[]card.Code{card.CodeOf(nil), card.CodeOf(errors.Join(errors.New("wrapped"), &card.Error{Code: card.CodeTimeout})), card.CodeOf(errors.New("plain"))})
}
