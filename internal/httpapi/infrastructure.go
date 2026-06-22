package httpapi

import (
	"net/http"

	"ai-pub/internal/app"
	"ai-pub/internal/domain"
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

// testServerConfig 对创建表单中的服务器配置做一次性连接校验，不落库。
func testServerConfig(service app.InfrastructureService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input domain.Server
		if !decodeJSON(w, r, &input) {
			return
		}
		result, err := service.TestServerConfig(r.Context(), input)
		if err != nil {
			writeError(w, r, http.StatusBadRequest, "ssh_test_failed", err)
			return
		}
		writeData(w, r, http.StatusOK, map[string]any{"result": result})
	}
}
