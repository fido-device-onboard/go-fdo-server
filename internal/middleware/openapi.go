package middleware

import (
	"log/slog"
	"net/http"

	swaggerui "github.com/swaggest/swgui/v5emb"
)

// ServeOpenAPI registers the OpenAPI spec endpoint and Swagger UI on the given mux.
func ServeOpenAPI(mux *http.ServeMux, title string, specJSON []byte) {
	mux.HandleFunc("GET /api/openapi.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write(specJSON); err != nil {
			slog.Error("Failed to write OpenAPI spec response", "error", err)
		}
	})

	mux.Handle("GET /api/docs/", swaggerui.New(
		title,
		"/api/openapi.json",
		"/api/docs/"))
}
