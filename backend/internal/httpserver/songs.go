package httpserver

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/7grecorder/7grecorder/backend/internal/account"
	"github.com/7grecorder/7grecorder/backend/internal/config"
	"github.com/7grecorder/7grecorder/backend/internal/db"
	"github.com/7grecorder/7grecorder/backend/internal/songs"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
)

func bindSongHandlers(cfg config.Config, s *ghttp.Server) {
	s.BindHandler("/api/v1/song-settings", func(r *ghttp.Request) {
		switch r.Method {
		case http.MethodGet:
			withSongStore(r, cfg, func(actor account.User, store songs.Store) {
				item, err := store.GetSettings(r.Context(), actor)
				if err != nil {
					writeSongError(r, err)
					return
				}
				r.Response.WriteJson(item)
			})
		case http.MethodPut:
			withSongStore(r, cfg, func(actor account.User, store songs.Store) {
				var req songs.SettingsUpsert
				if err := json.Unmarshal(r.GetBody(), &req); err != nil {
					writeAPIError(r, http.StatusBadRequest, "BAD_REQUEST", "Invalid JSON request body.", nil)
					return
				}
				item, err := store.UpsertSettings(r.Context(), actor, req)
				if err != nil {
					writeSongError(r, err)
					return
				}
				r.Response.WriteJson(item)
			})
		default:
			requireMethod(r, http.MethodGet)
		}
	})

	s.BindHandler("/api/v1/song-analysis/sources", func(r *ghttp.Request) {
		if !requireMethod(r, http.MethodGet) {
			return
		}
		withSongStore(r, cfg, func(actor account.User, store songs.Store) {
			items, err := store.ListSources(r.Context(), actor)
			if err != nil {
				writeSongError(r, err)
				return
			}
			r.Response.WriteJson(g.Map{"items": items, "page": 1, "page_size": len(items), "total": len(items)})
		})
	})

	s.BindHandler("/api/v1/song-analysis/runs", func(r *ghttp.Request) {
		switch r.Method {
		case http.MethodGet:
			withSongStore(r, cfg, func(actor account.User, store songs.Store) {
				items, err := store.ListRuns(r.Context(), actor)
				if err != nil {
					writeSongError(r, err)
					return
				}
				r.Response.WriteJson(g.Map{"items": items, "page": 1, "page_size": len(items), "total": len(items)})
			})
		case http.MethodPost:
			withSongStore(r, cfg, func(actor account.User, store songs.Store) {
				var req songs.CreateRunRequest
				if err := json.Unmarshal(r.GetBody(), &req); err != nil {
					writeAPIError(r, http.StatusBadRequest, "BAD_REQUEST", "Invalid JSON request body.", nil)
					return
				}
				item, err := store.CreateRun(r.Context(), actor, req)
				if err != nil {
					writeSongError(r, err)
					return
				}
				r.Response.Status = http.StatusCreated
				r.Response.WriteJson(item)
			})
		default:
			requireMethod(r, http.MethodGet)
		}
	})

	s.BindHandler("/api/v1/song-analysis/runs/{id}", func(r *ghttp.Request) {
		if !requireMethod(r, http.MethodGet) {
			return
		}
		withSongStore(r, cfg, func(actor account.User, store songs.Store) {
			item, err := store.GetRun(r.Context(), actor, r.Get("id").Int64())
			if err != nil {
				writeSongError(r, err)
				return
			}
			r.Response.WriteJson(item)
		})
	})
}

func withSongStore(r *ghttp.Request, cfg config.Config, fn func(account.User, songs.Store)) {
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
	fn(actor, songs.NewStore(database, cfg))
}

func writeSongError(r *ghttp.Request, err error) {
	switch {
	case errors.Is(err, songs.ErrForbidden):
		writeAPIError(r, http.StatusForbidden, "FORBIDDEN", "Songs requires SUPER_ADMIN.", nil)
	case errors.Is(err, songs.ErrValidation):
		writeAPIError(r, http.StatusBadRequest, "VALIDATION_FAILED", "Songs request is invalid.", nil)
	case errors.Is(err, songs.ErrNotReady):
		writeAPIError(r, http.StatusConflict, "SONGS_NOT_READY", "Songs settings or source is not ready.", nil)
	case errors.Is(err, songs.ErrNotFound):
		writeAPIError(r, http.StatusNotFound, "SONG_ANALYSIS_NOT_FOUND", "Song analysis run was not found.", nil)
	default:
		writeAPIError(r, http.StatusInternalServerError, "SONGS_OPERATION_FAILED", "Songs operation failed.", nil)
	}
}
