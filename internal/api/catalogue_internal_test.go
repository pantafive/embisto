package api

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/pantafive/embisto/card"
)

// TestEveryCodeAnswerableIsInTheCatalogue covers the codes the server names itself, besides the core's.
func TestEveryCodeAnswerableIsInTheCatalogue(t *testing.T) {
	known := func(code card.Code) bool {
		return slices.ContainsFunc(card.Catalogue(), func(entry card.CodeEntry) bool { return entry.Code == code })
	}
	var unknown []card.Code
	for _, refusal := range imageRefusals {
		if !known(refusal.code) {
			unknown = append(unknown, refusal.code)
		}
	}
	for _, code := range []card.Code{newProblem(400, "").(*problem).Code, newProblem(422, "").(*problem).Code, newProblem(500, "").(*problem).Code} {
		if !known(code) {
			unknown = append(unknown, code)
		}
	}

	assert.Empty(t, unknown)
}
