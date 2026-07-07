package httpapi

import (
	"errors"
	"net/http"

	"ai-pub/internal/app"
	"ai-pub/internal/domain"
	"ai-pub/internal/repository"
)

func testServer(service app.InfrastructureService, sshEnabled bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !sshEnabled {
			writeError(w, r, http.StatusBadRequest, "ssh_test_disabled", errors.New("ssh executor is disabled"))
			return
		}
		server, result, err := service.TestServer(r.Context(), r.PathValue("id"))
		if err != nil {
			writeError(w, r, http.StatusBadRequest, "ssh_test_failed", err)
			return
		}
		writeData(w, r, http.StatusOK, map[string]any{"server": server, "result": result})
	}
}

// testServerConfig 对创建表单中的服务器配置做一次性连接校验，不落库。
func testServerConfig(service app.InfrastructureService, sshEnabled bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !sshEnabled {
			writeError(w, r, http.StatusBadRequest, "ssh_test_disabled", errors.New("ssh executor is disabled"))
			return
		}
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

func listK8sClusters(service app.InfrastructureService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		items, err := service.ListK8sClusters(r.Context())
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "internal_error", err)
			return
		}
		writeData(w, r, http.StatusOK, items)
	}
}

func createK8sCluster(service app.InfrastructureService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input app.CreateK8sClusterInput
		if !decodeJSON(w, r, &input) {
			return
		}
		item, err := service.CreateK8sCluster(r.Context(), input)
		if err != nil {
			writeError(w, r, http.StatusBadRequest, "invalid_argument", err)
			return
		}
		writeData(w, r, http.StatusCreated, item)
	}
}

func patchK8sCluster(service app.InfrastructureService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input app.UpdateK8sClusterInput
		if !decodeJSON(w, r, &input) {
			return
		}
		item, err := service.UpdateK8sCluster(r.Context(), r.PathValue("id"), input)
		if err != nil {
			status := http.StatusBadRequest
			code := "invalid_argument"
			if errors.Is(err, repository.ErrNotFound) {
				status = http.StatusNotFound
				code = "not_found"
			}
			writeError(w, r, status, code, err)
			return
		}
		writeData(w, r, http.StatusOK, item)
	}
}

func deleteK8sCluster(service app.InfrastructureService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		err := service.DeleteK8sCluster(r.Context(), r.PathValue("id"))
		if err != nil {
			switch {
			case errors.Is(err, repository.ErrK8sClusterInUse):
				writeError(w, r, http.StatusConflict, "k8s_cluster_in_use", err)
			case errors.Is(err, repository.ErrNotFound):
				writeError(w, r, http.StatusNotFound, "not_found", err)
			default:
				writeError(w, r, http.StatusBadRequest, "invalid_argument", err)
			}
			return
		}
		writeData(w, r, http.StatusOK, map[string]bool{"deleted": true})
	}
}
