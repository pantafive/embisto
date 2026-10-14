// Command iframely-compare asks a running Iframely and a running embisto about the same URLs and reports where their
// answers differ in status, in the fields they carry, in the groups of their links and in their meta values. The raw
// answers are saved next to the report, so a difference can be read in full.
//
//	go run ./scripts/iframely-compare -iframely http://127.0.0.1:8061 -embisto http://127.0.0.1:8080
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
)

const requestTimeout = 30 * time.Second

// comparedMeta are the meta fields whose values are compared, not only their presence.
var comparedMeta = []string{"title", "site", "author", "canonical", "medium", "date"}

var notInName = regexp.MustCompile(`[^a-z0-9]+`)

type answer struct {
	status int
	body   any
	raw    []byte
}

type config struct {
	iframely, embisto, key, out string
}

func main() {
	var cfg config
	urls := flag.String("urls", filepath.Join("scripts", "iframely-compare", "urls.txt"), "the URLs to ask about, one a line")
	flag.StringVar(&cfg.iframely, "iframely", "http://127.0.0.1:8061", "a running Iframely")
	flag.StringVar(&cfg.embisto, "embisto", "http://127.0.0.1:8080", "a running embisto")
	flag.StringVar(&cfg.key, "key", "", "embisto's API key, when it requires one")
	flag.StringVar(&cfg.out, "out", "iframely-compare-out", "where the answers and report.md go")
	flag.Parse()
	if err := run(context.Background(), cfg, *urls); err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, cfg config, urlsFile string) error {
	raw, err := os.ReadFile(urlsFile) //nolint:gosec // the operator's own list
	if err != nil {
		return fmt.Errorf("reading the URLs: %w", err)
	}
	if err := os.MkdirAll(cfg.out, 0o750); err != nil {
		return fmt.Errorf("making the output directory: %w", err)
	}
	var report strings.Builder
	report.WriteString("| URL | /iframely | /oembed |\n|---|---|---|\n")
	for line := range strings.Lines(string(raw)) {
		target := strings.TrimSpace(line)
		if target == "" || strings.HasPrefix(target, "#") {
			continue
		}
		row, err := compareURL(ctx, cfg, target)
		if err != nil {
			return err
		}
		report.WriteString(row)
	}
	if err := os.WriteFile(filepath.Join(cfg.out, "report.md"), []byte(report.String()), 0o600); err != nil {
		return fmt.Errorf("saving the report: %w", err)
	}
	_, err = os.Stdout.WriteString(report.String())
	return err //nolint:wrapcheck // a failed print to the terminal
}

func compareURL(ctx context.Context, cfg config, target string) (string, error) {
	name := strings.Trim(notInName.ReplaceAllString(strings.ToLower(target), "-"), "-")
	cells := make([]string, 0, 2)
	for _, path := range []string{"iframely", "oembed"} {
		query := url.Values{"url": {target}}
		theirs, err := ask(ctx, cfg.iframely+"/"+path+"?group=true&"+query.Encode(), "")
		if err != nil {
			return "", err
		}
		ours, err := ask(ctx, cfg.embisto+"/"+path+"?"+query.Encode(), cfg.key)
		if err != nil {
			return "", err
		}
		for who, got := range map[string]answer{"iframely": theirs, "embisto": ours} {
			file := filepath.Join(cfg.out, fmt.Sprintf("%s.%s.%s.json", name, path, who))
			if err := os.WriteFile(file, got.raw, 0o600); err != nil {
				return "", fmt.Errorf("saving an answer: %w", err)
			}
		}
		cells = append(cells, differences(theirs, ours))
	}
	return fmt.Sprintf("| %s | %s | %s |\n", target, cells[0], cells[1]), nil
}

func ask(ctx context.Context, address, key string) (answer, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return answer{}, fmt.Errorf("asking %s: %w", address, err)
	}
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return answer{}, fmt.Errorf("asking %s: %w", address, err)
	}
	defer resp.Body.Close() //nolint:errcheck // a read-only body
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return answer{}, fmt.Errorf("reading %s: %w", address, err)
	}
	got := answer{status: resp.StatusCode, raw: raw}
	if err := json.Unmarshal(raw, &got.body); err != nil {
		got.body = nil
	}
	return got, nil
}

// differences says how ours differs from theirs, or that they match: the status, the fields only one carries, the
// link groups, and the compared meta values.
func differences(theirs, ours answer) string {
	var found []string
	if theirs.status != ours.status {
		found = append(found, fmt.Sprintf("status %d vs %d", theirs.status, ours.status))
	}
	theirFields, ourFields := shape(theirs.body), shape(ours.body)
	if missing := minus(theirFields, ourFields); len(missing) > 0 {
		found = append(found, "missing "+strings.Join(missing, ", "))
	}
	if extra := minus(ourFields, theirFields); len(extra) > 0 {
		found = append(found, "extra "+strings.Join(extra, ", "))
	}
	for _, field := range comparedMeta {
		theirValue, ourValue := metaValue(theirs.body, field), metaValue(ours.body, field)
		if theirValue != "" && ourValue != "" && theirValue != ourValue {
			found = append(found, field+" differs")
		}
	}
	if len(found) == 0 {
		return "match"
	}
	return strings.Join(found, "; ")
}

// shape is the set of a JSON value's field paths, with lists folded to [] and the meaningless option bags left out.
func shape(value any) map[string]bool {
	paths := map[string]bool{}
	var walk func(prefix string, value any)
	walk = func(prefix string, value any) {
		switch v := value.(type) {
		case map[string]any:
			for key, child := range v {
				if key == "options" {
					continue
				}
				path := strings.TrimPrefix(prefix+"."+key, ".")
				paths[path] = true
				walk(path, child)
			}
		case []any:
			for _, child := range v {
				walk(prefix+"[]", child)
			}
		}
	}
	walk("", value)
	return paths
}

func minus(a, b map[string]bool) []string {
	var only []string
	for _, path := range slices.Sorted(maps.Keys(a)) {
		// Each rel and media key of a link is reported once, under its group, not under every rel.
		if !b[path] && !strings.Contains(path, ".rel[]") {
			only = append(only, path)
		}
	}
	return only
}

func metaValue(body any, field string) string {
	object, ok := body.(map[string]any)
	if !ok {
		return ""
	}
	if meta, ok := object["meta"].(map[string]any); ok {
		object = meta
	}
	value, ok := object[field]
	if !ok {
		return ""
	}
	return fmt.Sprint(value)
}
