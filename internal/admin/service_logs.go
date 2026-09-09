package admin

import (
	"context"
	"net/http"
	"net/url"
	"time"

	"github.com/J0es1ick/Scheduler/internal/servicelogs"
)

type serviceLogReader interface {
	Read(context.Context, url.Values) (servicelogs.Page, error)
}

func (s *Server) handleServiceLogs(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if _, err := servicelogs.ParseQuery(r.URL.Query(), time.Now()); err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}
	if s.serviceLogs == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "Просмотр журналов не подключён")
		return
	}
	page, err := s.serviceLogs.Read(r.Context(), r.URL.Query())
	if err != nil {
		writeAPIError(w, http.StatusServiceUnavailable, "Журналы временно недоступны. Проверьте log-reader.")
		return
	}
	writeJSON(w, http.StatusOK, page)
}
