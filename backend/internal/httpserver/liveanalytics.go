package httpserver

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/7grecorder/7grecorder/backend/internal/account"
	"github.com/7grecorder/7grecorder/backend/internal/config"
	"github.com/7grecorder/7grecorder/backend/internal/db"
	"github.com/7grecorder/7grecorder/backend/internal/liveanalytics"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
)

func bindLiveAnalyticsHandlers(cfg config.Config, s *ghttp.Server) {
	s.BindHandler("/api/v1/live-analytics/sessions/{id}/raw-files", func(r *ghttp.Request) {
		if !requireMethod(r, http.MethodGet) {
			return
		}
		withLiveAnalyticsStore(r, cfg, func(actor account.User, store liveanalytics.Store) {
			result, err := store.RawFiles(r.Context(), actor, r.Get("id").Int64())
			if err != nil {
				writeLiveAnalyticsError(r, err)
				return
			}
			r.Response.WriteJson(result)
		})
	})

	s.BindHandler("/api/v1/live-analytics/sessions/{id}/timeline", func(r *ghttp.Request) {
		if !requireMethod(r, http.MethodGet) {
			return
		}
		withLiveAnalyticsStore(r, cfg, func(actor account.User, store liveanalytics.Store) {
			result, err := store.Timeline(r.Context(), actor, r.Get("id").Int64())
			if err != nil {
				writeLiveAnalyticsError(r, err)
				return
			}
			r.Response.WriteJson(result)
		})
	})

	s.BindHandler("/api/v1/live-analytics/sessions/{id}/events", func(r *ghttp.Request) {
		if !requireMethod(r, http.MethodGet) {
			return
		}
		withLiveAnalyticsStore(r, cfg, func(actor account.User, store liveanalytics.Store) {
			limit := r.Get("limit", 100).Int()
			page, err := store.Events(r.Context(), actor, r.Get("id").Int64(), r.Get("offset", 0).Int64(), limit, r.Get("file_id", 0).Int64())
			if err != nil {
				writeLiveAnalyticsError(r, err)
				return
			}
			r.Response.WriteJson(page)
		})
	})

	s.BindHandler("/api/v1/recording-profiles/{id}/live-analytics", func(r *ghttp.Request) {
		profileID := r.Get("id").Int64()
		switch r.Method {
		case http.MethodGet:
			withLiveAnalyticsStore(r, cfg, func(actor account.User, store liveanalytics.Store) {
				item, err := store.GetConfig(r.Context(), actor, profileID)
				if err != nil {
					writeLiveAnalyticsError(r, err)
					return
				}
				r.Response.WriteJson(item)
			})
		case http.MethodPut:
			withLiveAnalyticsStore(r, cfg, func(actor account.User, store liveanalytics.Store) {
				var req liveanalytics.ConfigUpsert
				if json.Unmarshal(r.GetBody(), &req) != nil {
					writeAPIError(r, http.StatusBadRequest, "BAD_REQUEST", "Invalid JSON request body.", nil)
					return
				}
				item, err := store.UpsertConfig(r.Context(), actor, profileID, req)
				if err != nil {
					writeLiveAnalyticsError(r, err)
					return
				}
				r.Response.WriteJson(item)
			})
		default:
			requireMethod(r, http.MethodGet)
		}
	})

	s.BindHandler("/api/v1/live-analytics/sessions", func(r *ghttp.Request) {
		if !requireMethod(r, http.MethodGet) {
			return
		}
		profileID := r.Get("profile_id").Int64()
		if profileID <= 0 {
			writeAPIError(r, http.StatusBadRequest, "VALIDATION_FAILED", "profile_id is required.", nil)
			return
		}
		withLiveAnalyticsStore(r, cfg, func(actor account.User, store liveanalytics.Store) {
			items, err := store.ListSessions(r.Context(), actor, profileID)
			if err != nil {
				writeLiveAnalyticsError(r, err)
				return
			}
			r.Response.WriteJson(g.Map{"items": items, "page": 1, "page_size": len(items), "total": len(items)})
		})
	})

	s.BindHandler("/api/v1/live-analytics/sessions/{id}", func(r *ghttp.Request) {
		if !requireMethod(r, http.MethodGet) {
			return
		}
		withLiveAnalyticsStore(r, cfg, func(actor account.User, store liveanalytics.Store) {
			item, err := store.GetSession(r.Context(), actor, r.Get("id").Int64())
			if err != nil {
				writeLiveAnalyticsError(r, err)
				return
			}
			r.Response.WriteJson(item)
		})
	})
}

func withLiveAnalyticsStore(r *ghttp.Request, cfg config.Config, fn func(account.User, liveanalytics.Store)) {
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
	fn(actor, liveanalytics.NewStore(database, cfg))
}

func writeLiveAnalyticsError(r *ghttp.Request, err error) {
	switch {
	case errors.Is(err, liveanalytics.ErrEvidenceUnavailable):
		writeAPIError(r, http.StatusGone, "EVIDENCE_UNAVAILABLE", "Raw evidence is unavailable or has been cleaned.", nil)
	case errors.Is(err, liveanalytics.ErrValidation):
		writeAPIError(r, http.StatusBadRequest, "VALIDATION_FAILED", "Live analytics request is invalid.", nil)
	case errors.Is(err, liveanalytics.ErrForbidden):
		writeAPIError(r, http.StatusForbidden, "FORBIDDEN", "Live analytics resource is not visible to this user.", nil)
	case errors.Is(err, liveanalytics.ErrNotFound):
		writeAPIError(r, http.StatusNotFound, "LIVE_ANALYTICS_NOT_FOUND", "Live analytics resource was not found.", nil)
	default:
		writeAPIError(r, http.StatusInternalServerError, "LIVE_ANALYTICS_OPERATION_FAILED", "Live analytics operation failed.", nil)
	}
}
