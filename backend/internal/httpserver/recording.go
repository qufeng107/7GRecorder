package httpserver

import (
	"errors"
	"net/http"

	"github.com/7grecorder/7grecorder/backend/internal/account"
	"github.com/7grecorder/7grecorder/backend/internal/config"
	"github.com/7grecorder/7grecorder/backend/internal/db"
	"github.com/7grecorder/7grecorder/backend/internal/recording"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
)

func bindRecordingHandlers(cfg config.Config, s *ghttp.Server) {
	s.BindHandler("/api/v1/recordings", func(r *ghttp.Request) {
		if !requireMethod(r, http.MethodGet) {
			return
		}
		withRecordingStore(r, cfg, func(actor account.User, store recording.Store) {
			items, err := store.List(r.Context(), actor)
			if err != nil {
				writeRecordingError(r, err)
				return
			}
			r.Response.WriteJson(g.Map{"items": items, "page": 1, "page_size": len(items), "total": len(items)})
		})
	})

	s.BindHandler("/api/v1/recording-files/reconcile", func(r *ghttp.Request) {
		if !requireMethod(r, http.MethodPost) {
			return
		}
		withRecordingStore(r, cfg, func(actor account.User, store recording.Store) {
			result, err := store.ReconcileLocal(r.Context(), actor)
			if err != nil {
				writeRecordingError(r, err)
				return
			}
			r.Response.WriteJson(result)
		})
	})
}

func withRecordingStore(r *ghttp.Request, cfg config.Config, fn func(account.User, recording.Store)) {
	database, err := db.Open(r.Context(), cfg)
	if err != nil {
		writeAPIError(r, http.StatusInternalServerError, "DATABASE_UNAVAILABLE", "Database is unavailable.", nil)
		return
	}
	defer database.Close()

	actor, err := account.NewStore(database).UserBySessionToken(r.Context(), r.Cookie.Get(sessionCookieName).String())
	if errors.Is(err, account.ErrNotFound) || errors.Is(err, account.ErrDisabledUser) {
		writeAPIError(r, http.StatusUnauthorized, "NOT_AUTHENTICATED", "Login is required.", nil)
		return
	}
	if err != nil {
		writeAPIError(r, http.StatusInternalServerError, "SESSION_LOOKUP_FAILED", "Session lookup failed.", nil)
		return
	}
	fn(actor, recording.NewStore(database, cfg))
}

func writeRecordingError(r *ghttp.Request, err error) {
	switch {
	case errors.Is(err, recording.ErrForbidden):
		writeAPIError(r, http.StatusForbidden, "FORBIDDEN", "Recording operation is not allowed.", nil)
	default:
		writeAPIError(r, http.StatusInternalServerError, "RECORDING_OPERATION_FAILED", "Recording operation failed.", nil)
	}
}
