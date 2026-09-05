package httpserver

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/7grecorder/7grecorder/backend/internal/account"
	"github.com/7grecorder/7grecorder/backend/internal/config"
	"github.com/7grecorder/7grecorder/backend/internal/db"
	"github.com/7grecorder/7grecorder/backend/internal/profile"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
)

func bindProfileHandlers(cfg config.Config, s *ghttp.Server) {
	s.BindHandler("/api/v1/recording-profiles", func(r *ghttp.Request) {
		switch r.Method {
		case http.MethodGet:
			withProfileStore(r, cfg, func(actor account.User, store profile.Store) {
				items, err := store.List(r.Context(), actor)
				if err != nil {
					writeAPIError(r, http.StatusInternalServerError, "PROFILE_LIST_FAILED", "Could not list profiles.", nil)
					return
				}
				r.Response.WriteJson(g.Map{"items": items, "page": 1, "page_size": len(items), "total": len(items)})
			})
		case http.MethodPost:
			withProfileStore(r, cfg, func(actor account.User, store profile.Store) {
				var req profile.CreateRequest
				if err := json.Unmarshal(r.GetBody(), &req); err != nil {
					writeAPIError(r, http.StatusBadRequest, "BAD_REQUEST", "Invalid JSON request body.", nil)
					return
				}
				created, err := store.Create(r.Context(), actor, req)
				if err != nil {
					writeProfileError(r, err)
					return
				}
				r.Response.Status = http.StatusCreated
				r.Response.WriteJson(created)
			})
		default:
			requireMethod(r, http.MethodGet)
		}
	})

	s.BindHandler("/api/v1/recording-profiles/{id}", func(r *ghttp.Request) {
		id := r.Get("id").Int64()
		switch r.Method {
		case http.MethodGet:
			withProfileStore(r, cfg, func(actor account.User, store profile.Store) {
				item, err := store.Get(r.Context(), actor, id)
				if err != nil {
					writeProfileError(r, err)
					return
				}
				r.Response.WriteJson(item)
			})
		case http.MethodPatch:
			withProfileStore(r, cfg, func(actor account.User, store profile.Store) {
				var req profile.UpdateRequest
				if err := json.Unmarshal(r.GetBody(), &req); err != nil {
					writeAPIError(r, http.StatusBadRequest, "BAD_REQUEST", "Invalid JSON request body.", nil)
					return
				}
				updated, err := store.Update(r.Context(), actor, id, req)
				if err != nil {
					writeProfileError(r, err)
					return
				}
				r.Response.WriteJson(updated)
			})
		default:
			requireMethod(r, http.MethodGet)
		}
	})

	s.BindHandler("/api/v1/recording-profiles/{id}/runtime", func(r *ghttp.Request) {
		if !requireMethod(r, http.MethodGet) {
			return
		}
		id := r.Get("id").Int64()
		withProfileStore(r, cfg, func(actor account.User, store profile.Store) {
			item, err := store.Get(r.Context(), actor, id)
			if err != nil {
				writeProfileError(r, err)
				return
			}
			r.Response.WriteJson(item.Runtime)
		})
	})

	s.BindHandler("/api/v1/recording-profiles/{id}/recording-settings", func(r *ghttp.Request) {
		if !requireMethod(r, http.MethodPut) {
			return
		}
		id := r.Get("id").Int64()
		withProfileStore(r, cfg, func(actor account.User, store profile.Store) {
			var req profile.SettingsUpsert
			if err := json.Unmarshal(r.GetBody(), &req); err != nil {
				writeAPIError(r, http.StatusBadRequest, "BAD_REQUEST", "Invalid JSON request body.", nil)
				return
			}
			settings, err := store.UpsertSettings(r.Context(), actor, id, req)
			if err != nil {
				writeProfileError(r, err)
				return
			}
			r.Response.WriteJson(settings)
		})
	})
}

func withProfileStore(r *ghttp.Request, cfg config.Config, fn func(account.User, profile.Store)) {
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
	fn(actor, profile.NewStore(database))
}

func writeProfileError(r *ghttp.Request, err error) {
	switch {
	case errors.Is(err, profile.ErrValidation):
		writeAPIError(r, http.StatusBadRequest, "VALIDATION_FAILED", "Profile request is invalid.", nil)
	case errors.Is(err, profile.ErrForbidden):
		writeAPIError(r, http.StatusForbidden, "FORBIDDEN", "Profile is not visible to this user.", nil)
	case errors.Is(err, profile.ErrNotFound):
		writeAPIError(r, http.StatusNotFound, "PROFILE_NOT_FOUND", "Profile was not found.", nil)
	case errors.Is(err, profile.ErrRoomInUse):
		writeAPIError(r, http.StatusConflict, "ROOM_IN_USE", "Room is already configured by another active profile.", nil)
	case errors.Is(err, profile.ErrManagerLimit):
		writeAPIError(r, http.StatusConflict, "MANAGER_PROFILE_LIMIT", "Manager already has an active profile.", nil)
	case errors.Is(err, profile.ErrRecordingActive):
		writeAPIError(r, http.StatusConflict, "RECORDING_ACTIVE", "Recording profile cannot change room while a recording is active.", nil)
	default:
		writeAPIError(r, http.StatusInternalServerError, "PROFILE_OPERATION_FAILED", "Profile operation failed.", nil)
	}
}
