package httpserver

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/7grecorder/7grecorder/backend/internal/account"
	"github.com/7grecorder/7grecorder/backend/internal/config"
	"github.com/7grecorder/7grecorder/backend/internal/db"
	"github.com/7grecorder/7grecorder/backend/internal/sitetls"
	"github.com/gogf/gf/v2/net/ghttp"
)

func bindSiteTLSHandlers(cfg config.Config, s *ghttp.Server) {
	s.BindHandler("/api/v1/system/site-tls", func(r *ghttp.Request) {
		switch r.Method {
		case http.MethodGet:
			withSiteTLSStore(r, cfg, func(actor account.User, store sitetls.Store) {
				settings, err := store.Get(r.Context(), actor)
				if err != nil {
					writeSiteTLSError(r, err)
					return
				}
				r.Response.WriteJson(settings)
			})
		case http.MethodPut:
			withSiteTLSStore(r, cfg, func(actor account.User, store sitetls.Store) {
				var req sitetls.SettingsUpsert
				if err := json.Unmarshal(r.GetBody(), &req); err != nil {
					writeAPIError(r, http.StatusBadRequest, "BAD_REQUEST", "Invalid JSON request body.", nil)
					return
				}
				settings, err := store.Upsert(r.Context(), actor, req)
				if err != nil {
					writeSiteTLSError(r, err)
					return
				}
				r.Response.WriteJson(settings)
			})
		default:
			requireMethod(r, http.MethodGet)
		}
	})

	s.BindHandler("/api/v1/system/site-tls/actions/sync", func(r *ghttp.Request) {
		if !requireMethod(r, http.MethodPost) {
			return
		}
		withSiteTLSStore(r, cfg, func(actor account.User, store sitetls.Store) {
			settings, err := store.ScheduleNow(r.Context(), actor)
			if err != nil {
				writeSiteTLSError(r, err)
				return
			}
			r.Response.WriteJson(settings)
		})
	})
}

func withSiteTLSStore(r *ghttp.Request, cfg config.Config, fn func(account.User, sitetls.Store)) {
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
	fn(actor, sitetls.NewStore(database, cfg))
}

func writeSiteTLSError(r *ghttp.Request, err error) {
	switch {
	case errors.Is(err, sitetls.ErrForbidden):
		writeAPIError(r, http.StatusForbidden, "FORBIDDEN", "Site TLS settings require SUPER_ADMIN.", nil)
	case errors.Is(err, sitetls.ErrValidation):
		writeAPIError(r, http.StatusBadRequest, "VALIDATION_FAILED", "Site TLS settings are invalid.", nil)
	case errors.Is(err, sitetls.ErrNotFound):
		writeAPIError(r, http.StatusNotFound, "SITE_TLS_NOT_FOUND", "Site TLS settings were not found.", nil)
	default:
		writeAPIError(r, http.StatusInternalServerError, "SITE_TLS_OPERATION_FAILED", "Site TLS operation failed.", nil)
	}
}
