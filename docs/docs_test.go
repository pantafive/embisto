package docs_test

import (
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pantafive/embisto/card"
	"github.com/pantafive/embisto/docs"
)

// cardFields are the card's JSON fields by path, such as images[].url, each with the struct and field it is; a nested
// type reached by several paths, as icon, image and images are, lists its fields under each.
func cardFields(t reflect.Type, prefix string, found map[string]string) {
	for field := range t.Fields() {
		name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
		if name == "" || name == "-" {
			continue
		}
		path := prefix + name
		found[path] = t.Name() + "." + name
		nested := field.Type
		if nested.Kind() == reflect.Slice {
			nested, path = nested.Elem(), path+"[]"
		}
		if nested.Kind() == reflect.Struct && nested != reflect.TypeFor[time.Time]() {
			cardFields(nested, path+".", found)
		}
	}
}

// documented are the paths the card table of api.md names in its first column.
func documented(t *testing.T) []string {
	t.Helper()
	page, err := docs.Files.ReadFile("api.md")
	require.NoError(t, err)
	_, table, ok := strings.Cut(string(page), "| Field | Type | Meaning |\n|---|---|---|\n")
	require.True(t, ok, "api.md has no card table")
	table, _, _ = strings.Cut(table, "\n\n")
	var paths []string
	for row := range strings.Lines(table) {
		first := strings.Split(row, "|")[1]
		for _, match := range regexp.MustCompile("`([^`]+)`").FindAllStringSubmatch(first, -1) {
			paths = append(paths, match[1])
		}
	}
	return paths
}

// TestAPIDocumentsEveryCardField holds api.md to the card: each field of each type is documented under one of the
// paths that reach it, and every documented path is a field.
func TestAPIDocumentsEveryCardField(t *testing.T) {
	fields := map[string]string{}
	cardFields(reflect.TypeFor[card.Preview](), "", fields)

	paths := documented(t)

	var missing, unknown []string
	for _, field := range fields {
		if !slices.Contains(missing, field) && !slices.ContainsFunc(paths, func(path string) bool { return fields[path] == field }) {
			missing = append(missing, field)
		}
	}
	for _, path := range paths {
		if _, ok := fields[path]; !ok {
			unknown = append(unknown, path)
		}
	}
	slices.Sort(missing)
	assert.Equal(t, []any{[]string(nil), []string(nil)}, []any{missing, unknown}, "card fields missing from api.md, and paths api.md names that are not fields")
}
