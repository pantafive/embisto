package sites

import (
	"errors"
	"slices"

	"github.com/pantafive/embisto/card"
)

// schemeHTTPS is the scheme of every address the providers make.
const schemeHTTPS = "https"

// gone is an oEmbed failure whose status, among those named, says the item does not exist: not_found, which the
// pipeline answers as such rather than with another source's card. Any other failure is returned as it was.
func gone(err error, statuses ...int) error {
	var failed *card.Error
	if errors.As(err, &failed) && slices.Contains(statuses, failed.Status) {
		return &card.Error{Code: card.CodeNotFound, Status: failed.Status, Err: err}
	}
	return err
}
