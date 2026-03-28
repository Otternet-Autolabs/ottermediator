package api

import (
	"encoding/json"
	"io"
	"log"
	"mime"
	"net/http"
	"path/filepath"

	"github.com/i574789/ottermediator/chromecast"
	"github.com/i574789/ottermediator/config"
)

type Handler struct {
	dm  *chromecast.DiscoveryManager
	cfg *config.Config
}

func NewHandler(dm *chromecast.DiscoveryManager, cfg *config.Config) *Handler {
	return &Handler{dm: dm, cfg: cfg}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux, hub *Hub) {
	mux.HandleFunc("GET /api/devices", h.listDevices)
	mux.HandleFunc("GET /api/preview", h.preview)
	mux.HandleFunc("POST /api/devices/{id}/play", h.play)
	mux.HandleFunc("POST /api/devices/{id}/pause", h.pause)
	mux.HandleFunc("POST /api/devices/{id}/stop", h.stop)
	mux.HandleFunc("POST /api/devices/{id}/seek", h.seek)
	mux.HandleFunc("POST /api/devices/{id}/volume", h.volume)
	mux.HandleFunc("POST /api/devices/{id}/next", h.next)
	mux.HandleFunc("POST /api/devices/{id}/prev", h.prev)
	mux.HandleFunc("POST /api/devices/{id}/cast", h.cast)
	mux.HandleFunc("POST /api/devices/{id}/cast-site", h.castSite)
	mux.HandleFunc("POST /api/devices/{id}/cast-auto", h.castAuto)
	mux.HandleFunc("PUT /api/devices/{id}/keepalive", h.setKeepalive)
	mux.HandleFunc("GET /ws", hub.ServeWS)
}

func (h *Handler) preview(w http.ResponseWriter, r *http.Request) {
	pageURL := r.URL.Query().Get("url")
	if pageURL == "" {
		http.Error(w, "missing url", http.StatusBadRequest)
		return
	}
	imageURL := PreviewImage(pageURL)
	if imageURL == "" {
		http.NotFound(w, r)
		return
	}
	// Proxy the image to avoid CORS issues
	resp, err := httpClient.Get(imageURL)
	if err != nil || resp.StatusCode != 200 {
		http.NotFound(w, r)
		return
	}
	defer resp.Body.Close()
	w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
	w.Header().Set("Cache-Control", "max-age=600")
	io.Copy(w, resp.Body)
}

func (h *Handler) listDevices(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, h.dm.AllStatuses())
}

func (h *Handler) play(w http.ResponseWriter, r *http.Request) {
	dev, ok := h.device(w, r)
	if !ok {
		return
	}
	if err := dev.Play(); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, dev.GetStatus())
}

func (h *Handler) pause(w http.ResponseWriter, r *http.Request) {
	dev, ok := h.device(w, r)
	if !ok {
		return
	}
	if err := dev.Pause(); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, dev.GetStatus())
}

func (h *Handler) stop(w http.ResponseWriter, r *http.Request) {
	dev, ok := h.device(w, r)
	if !ok {
		return
	}
	if err := dev.Stop(); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, dev.GetStatus())
}

func (h *Handler) seek(w http.ResponseWriter, r *http.Request) {
	dev, ok := h.device(w, r)
	if !ok {
		return
	}
	var body struct {
		Position float32 `json:"position"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	if err := dev.Seek(body.Position); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, dev.GetStatus())
}

func (h *Handler) volume(w http.ResponseWriter, r *http.Request) {
	dev, ok := h.device(w, r)
	if !ok {
		return
	}
	var body struct {
		Level float32 `json:"level"`
		Muted bool    `json:"muted"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	if err := dev.SetVolume(body.Level, body.Muted); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, dev.GetStatus())
}

func (h *Handler) next(w http.ResponseWriter, r *http.Request) {
	dev, ok := h.device(w, r)
	if !ok {
		return
	}
	if err := dev.Next(); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, dev.GetStatus())
}

func (h *Handler) prev(w http.ResponseWriter, r *http.Request) {
	dev, ok := h.device(w, r)
	if !ok {
		return
	}
	if err := dev.Prev(); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, dev.GetStatus())
}

func (h *Handler) cast(w http.ResponseWriter, r *http.Request) {
	dev, ok := h.device(w, r)
	if !ok {
		return
	}
	var body struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.URL == "" {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	if err := dev.Cast(body.URL); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, dev.GetStatus())
}

func (h *Handler) castSite(w http.ResponseWriter, r *http.Request) {
	dev, ok := h.device(w, r)
	if !ok {
		return
	}
	var body struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.URL == "" {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	if err := dev.CastSite(body.URL); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, map[string]string{"status": "ok"})
}

func (h *Handler) castAuto(w http.ResponseWriter, r *http.Request) {
	dev, ok := h.device(w, r)
	if !ok {
		return
	}
	var body struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.URL == "" {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	var err error
	if mime.TypeByExtension(filepath.Ext(body.URL)) == "" {
		err = dev.CastSite(body.URL)
	} else {
		err = dev.Cast(body.URL)
	}
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, map[string]string{"status": "ok"})
}

func (h *Handler) setKeepalive(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		Mode config.KeepaliveMode `json:"mode"`
		URL  string               `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	dc := config.DeviceConfig{
		KeepaliveMode: body.Mode,
		KeepaliveURL:  body.URL,
	}
	if err := h.cfg.SetDevice(id, dc); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, map[string]string{"status": "ok"})
}

// ── helpers ──────────────────────────────────────────────────────────────────

func (h *Handler) device(w http.ResponseWriter, r *http.Request) (*chromecast.Device, bool) {
	id := r.PathValue("id")
	dev, ok := h.dm.GetDevice(id)
	if !ok {
		http.Error(w, "device not found", http.StatusNotFound)
		return nil, false
	}
	return dev, true
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, err error) {
	log.Printf("API error: %v", err)
	http.Error(w, err.Error(), http.StatusInternalServerError)
}
