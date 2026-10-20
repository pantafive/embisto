package main

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pantafive/embisto"
)

func TestRunStopsOnATemplateThatCannotServe(t *testing.T) {
	tests := []struct {
		name, file, source, says string
	}{
		{"a parse error", "broken.txt.tmpl", "{{.Title", "broken.txt.tmpl"},
		{"JSON without escaping", "slack.json.tmpl", `{"text": "{{.Title}}"}`, "slack.json.tmpl: on a card of hostile text, the output is not JSON"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(dir, tt.file), []byte(tt.source), 0o600))

			err := run(context.Background(), []string{"embisto"}, func() []string {
				return []string{"EMBISTO_TEMPLATES_DIR=" + dir, "EMBISTO_ADDR=127.0.0.1:0", "EMBISTO_METRICS_ADDR=127.0.0.1:0"}
			}, io.Discard)

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.says)
		})
	}
}

func TestRunPrintsUsageAndRefusesUnknownCommands(t *testing.T) {
	// Each environment is one the server would refuse, so a command taken for the server fails rather than serves.
	refused := func() []string { return []string{"EMBISTO_LOG_LEVEL=loud"} }
	for _, arg := range []string{"help", "-h", "--help"} {
		t.Run(arg, func(t *testing.T) {
			var out strings.Builder

			err := run(t.Context(), []string{"embisto", arg}, refused, &out)

			require.NoError(t, err)
			assert.Contains(t, out.String(), "docs/configuration.md")
		})
	}
	t.Run("an unknown command", func(t *testing.T) {
		var out strings.Builder

		err := run(t.Context(), []string{"embisto", "--port=80"}, refused, &out)

		require.ErrorIs(t, err, errUnknownCommand)
		assert.Equal(t, []any{true, ""}, []any{strings.Contains(err.Error(), "embisto sign"), out.String()})
	})
}

func TestSignPrintsAURLOnlyWithThePublicAddress(t *testing.T) {
	key := "EMBISTO_IMAGE_KEY=" + strings.Repeat("k", 32)
	sign := func(env ...string) (string, error) {
		var out strings.Builder
		err := run(t.Context(), []string{"embisto", "sign", "card", "https://example.com/a.png"}, func() []string { return env }, &out)
		return out.String(), err
	}

	signed, err := sign(key, "EMBISTO_PUBLIC_URL=https://embisto.example.com/")
	require.NoError(t, err)
	_, err = sign(key)

	require.ErrorIs(t, err, errNoPublicURL)
	assert.Regexp(t, `^https://embisto\.example\.com/i/[^/]+/card/[^/]+\n$`, signed)
}

func TestServerTimeoutsOutlastTheSlowestAnswer(t *testing.T) {
	server := newServer(":0", http.NotFoundHandler())

	slowestPreview := lockTTL + embisto.Defaults().Timeout
	slowestPicture := imageWait + imageTimeout
	slowest := max(slowestPreview, slowestPicture)
	assert.Equal(t, []bool{true, true, true, true, true, true}, []bool{
		server.ReadHeaderTimeout > 0, server.ReadTimeout > 0, server.IdleTimeout > 0,
		server.WriteTimeout > slowest,
		// An answer begun just before the server stops taking requests still finishes, and the process exits before
		// Kubernetes' default grace of 30 seconds runs out.
		shutdownGrace > slowest, unreadyFor+shutdownGrace < 30*time.Second,
	})
}
