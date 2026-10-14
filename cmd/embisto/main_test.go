package main

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"

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
			})

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.says)
		})
	}
}

func TestServerTimeoutsOutlastTheSlowestAnswer(t *testing.T) {
	server := newServer(":0", http.NotFoundHandler())

	slowestPreview := lockTTL + embisto.Defaults().Timeout
	slowestPicture := imageWait + imageTimeout
	assert.Equal(t, []bool{true, true, true, true}, []bool{
		server.ReadHeaderTimeout > 0, server.ReadTimeout > 0, server.IdleTimeout > 0,
		server.WriteTimeout > max(slowestPreview, slowestPicture),
	})
}
