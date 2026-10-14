package api

import (
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/pantafive/embisto/card"
)

// problem is huma's RFC 9457 error model with the machine code.
type problem struct {
	huma.ErrorModel

	Code card.Code `json:"code" doc:"What failed, for a client to branch on"`

	headers http.Header
}

// TransformSchema lists the catalogue's codes as the code's enum, so the spec never drifts from what is answered.
func (*problem) TransformSchema(_ huma.Registry, schema *huma.Schema) *huma.Schema {
	if code, ok := schema.Properties["code"]; ok {
		code.Enum = nil
		for _, entry := range card.Catalogue() {
			code.Enum = append(code.Enum, string(entry.Code))
		}
	}
	return schema
}

// GetHeaders are the headers huma sets on the refusal, such as Cache-Control.
func (p *problem) GetHeaders() http.Header {
	return p.headers
}

// problemOf is a failure as problem+json: its status and words come from the catalogue.
func problemOf(failed *card.Error) *problem {
	status := failed.HTTPStatus()
	return &problem{Status: status, Title: http.StatusText(status), Detail: failed.Error(), Code: failed.Code}
}

// refusal is the problem of a code alone.
func refusal(code card.Code) *problem {
	return problemOf(&card.Error{Code: code})
}

// humaError is huma's own constructor, kept before newProblem replaces it.
var humaError = huma.NewError

// newProblem replaces huma's error constructor, so the errors huma raises itself carry a code from the catalogue: a
// format the server does not have is unknown_format, any other malformed parameter invalid_request. huma's own list
// of what failed, which only repeats the request, is kept.
func newProblem(status int, detail string, errs ...error) huma.StatusError {
	if status >= http.StatusInternalServerError {
		return refusal(card.CodeInternal)
	}
	code := card.CodeInvalidRequest
	refused := refusal(code)
	if model, ok := humaError(status, detail, errs...).(*huma.ErrorModel); ok {
		for _, found := range model.Errors {
			if found != nil && found.Location == "query.format" {
				code = card.CodeUnknownFormat
			}
		}
		refused = refusal(code)
		refused.Errors = model.Errors
	}
	return refused
}
