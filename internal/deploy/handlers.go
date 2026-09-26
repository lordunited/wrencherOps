package deploy

import (
	"encoding/json"
	"net/http"
)

type Handler struct {
	store *Store
}

func NewHandler(store *Store) *Handler {
	return &Handler{store: store}
}

// RegisterRoutes wires job polling, shared by every service type's deploy
// flow. Starting a deploy happens through the service that owns the
// config (see internal/service), not here.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/deploy/jobs/{id}", h.handleGetJob)
}

func (h *Handler) handleGetJob(w http.ResponseWriter, r *http.Request) {
	job, ok := h.store.Get(r.PathValue("id"))
	if !ok {
		http.Error(w, "job not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(job.Snapshot())
}
