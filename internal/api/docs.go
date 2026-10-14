package api

import (
	"io"
	"log/slog"
	"net/http"
)

// referencePage is the API reference: Scalar's standalone script at a pinned version from its CDN, checked against the
// file's hash, reading the spec from this server. Scalar's own proxy, telemetry, fonts, AI agent and MCP link are
// turned off.
const referencePage = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>embisto API reference</title>
</head>
<body>
<div id="reference"></div>
<script src="https://cdn.jsdelivr.net/npm/@scalar/api-reference@1.72.1/dist/browser/standalone.js"
  integrity="sha384-U11tb2XnKvmwt8RlTvnwUnYgrN+ur4Xyh9htLhjajWNR/Oyl5AX5DEz00qRmlrmK" crossorigin="anonymous"></script>
<script>
Scalar.createApiReference('#reference', {
  url: 'openapi.json',
  proxyUrl: '',
  telemetry: false,
  withDefaultFonts: false,
  showDeveloperTools: 'never',
  agent: { disabled: true },
  mcp: { disabled: true },
  hideClientButton: true,
  documentDownloadType: 'json',
})
</script>
</body>
</html>
`

// addReference serves the API reference at /docs.
func addReference(mux *http.ServeMux, log *slog.Logger) {
	mux.HandleFunc("GET /docs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if _, err := io.WriteString(w, referencePage); err != nil {
			log.WarnContext(r.Context(), "the reference page was not written", slog.Any("error", err))
		}
	})
}
