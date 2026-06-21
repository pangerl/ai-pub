package httpapi

import (
	"net/http"

	"ai-pub/internal/app"
)

func testServer(service app.InfrastructureService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		server, result, err := service.TestServer(r.Context(), r.PathValue("id"))
		if err != nil {
			writeError(w, r, http.StatusBadRequest, "ssh_test_failed", err)
			return
		}
		writeData(w, r, http.StatusOK, map[string]any{"server": server, "result": result})
	}
}
