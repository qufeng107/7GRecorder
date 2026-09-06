package httpserver

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/7grecorder/7grecorder/backend/internal/account"
	"github.com/7grecorder/7grecorder/backend/internal/config"
	"github.com/7grecorder/7grecorder/backend/internal/db"
	"github.com/7grecorder/7grecorder/backend/internal/recording"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
)

func bindRecordingHandlers(cfg config.Config, s *ghttp.Server) {
	s.BindHandler("/api/v1/storage/local", func(r *ghttp.Request) {
		switch r.Method {
		case http.MethodGet:
			withRecordingStore(r, cfg, func(actor account.User, store recording.Store) {
				status, err := store.LocalStorageStatus(r.Context(), actor)
				if err != nil {
					writeRecordingError(r, err)
					return
				}
				r.Response.WriteJson(status)
			})
		case http.MethodPut:
			writeLocalStorageSettings(r, cfg)
		default:
			requireMethod(r, http.MethodGet)
		}
	})

	s.BindHandler("/api/v1/storage/local/settings", func(r *ghttp.Request) {
		if !requireMethod(r, http.MethodPut) {
			return
		}
		writeLocalStorageSettings(r, cfg)
	})

	s.BindHandler("/api/v1/storage/local/cleanup-candidates", func(r *ghttp.Request) {
		if !requireMethod(r, http.MethodGet) {
			return
		}
		withRecordingStore(r, cfg, func(actor account.User, store recording.Store) {
			result, err := store.CleanupCandidates(r.Context(), actor, r.Get("limit").Int())
			if err != nil {
				writeRecordingError(r, err)
				return
			}
			r.Response.WriteJson(result)
		})
	})

	s.BindHandler("/api/v1/storage/local/actions/cleanup", func(r *ghttp.Request) {
		if !requireMethod(r, http.MethodPost) {
			return
		}
		withRecordingStore(r, cfg, func(actor account.User, store recording.Store) {
			var req recording.CleanupRunRequest
			if len(r.GetBody()) > 0 {
				if err := json.Unmarshal(r.GetBody(), &req); err != nil {
					writeAPIError(r, http.StatusBadRequest, "BAD_REQUEST", "Invalid JSON request body.", nil)
					return
				}
			}
			result, err := store.RunLocalCleanup(r.Context(), actor, req)
			if err != nil {
				writeRecordingError(r, err)
				return
			}
			r.Response.WriteJson(result)
		})
	})

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

	s.BindHandler("/api/v1/recordings/{id}/actions/protect-local", func(r *ghttp.Request) {
		if !requireMethod(r, http.MethodPost) {
			return
		}
		id := r.Get("id").Int64()
		withRecordingStore(r, cfg, func(actor account.User, store recording.Store) {
			item, err := store.SetLocalProtected(r.Context(), actor, id, true)
			if err != nil {
				writeRecordingError(r, err)
				return
			}
			r.Response.WriteJson(item)
		})
	})

	s.BindHandler("/api/v1/recordings/{id}/actions/unprotect-local", func(r *ghttp.Request) {
		if !requireMethod(r, http.MethodPost) {
			return
		}
		id := r.Get("id").Int64()
		withRecordingStore(r, cfg, func(actor account.User, store recording.Store) {
			item, err := store.SetLocalProtected(r.Context(), actor, id, false)
			if err != nil {
				writeRecordingError(r, err)
				return
			}
			r.Response.WriteJson(item)
		})
	})

	s.BindHandler("/api/v1/recording-files/{id}/download", func(r *ghttp.Request) {
		if !requireMethod(r, http.MethodGet) {
			return
		}
		id := r.Get("id").Int64()
		withRecordingStore(r, cfg, func(actor account.User, store recording.Store) {
			file, err := store.FileForDownload(r.Context(), actor, id)
			if err != nil {
				writeRecordingError(r, err)
				return
			}
			if !strings.HasPrefix(filepath.ToSlash(file.RelativePath), "recordings/") {
				writeAPIError(r, http.StatusNotFound, "RECORDING_FILE_NOT_FOUND", "Recording file was not found.", nil)
				return
			}
			absolutePath, err := ResolveWithinRoot(cfg.DataRoot, file.RelativePath)
			if err != nil {
				writeAPIError(r, http.StatusNotFound, "RECORDING_FILE_NOT_FOUND", "Recording file was not found.", nil)
				return
			}
			info, err := os.Stat(absolutePath)
			if errors.Is(err, os.ErrNotExist) || (err == nil && info.IsDir()) {
				writeAPIError(r, http.StatusNotFound, "RECORDING_FILE_NOT_FOUND", "Recording file was not found.", nil)
				return
			}
			if err != nil {
				writeAPIError(r, http.StatusInternalServerError, "RECORDING_FILE_UNAVAILABLE", "Recording file is unavailable.", nil)
				return
			}

			r.Response.Header().Set("Content-Type", recordingContentType(file.OriginalName))
			r.Response.Header().Set("Content-Disposition", contentDisposition(file.OriginalName))
			r.Response.Header().Set("X-Accel-Redirect", protectedMediaPath(file.RelativePath))
		})
	})
}

func writeLocalStorageSettings(r *ghttp.Request, cfg config.Config) {
	withRecordingStore(r, cfg, func(actor account.User, store recording.Store) {
		var req recording.LocalStorageSettingsUpsert
		if err := json.Unmarshal(r.GetBody(), &req); err != nil {
			writeAPIError(r, http.StatusBadRequest, "BAD_REQUEST", "Invalid JSON request body.", nil)
			return
		}
		settings, err := store.UpsertLocalStorageSettings(r.Context(), actor, req)
		if err != nil {
			writeRecordingError(r, err)
			return
		}
		r.Response.WriteJson(settings)
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
	case errors.Is(err, recording.ErrNotFound):
		writeAPIError(r, http.StatusNotFound, "RECORDING_NOT_FOUND", "Recording was not found.", nil)
	case errors.Is(err, recording.ErrNotReady):
		writeAPIError(r, http.StatusConflict, "RECORDING_FILE_NOT_READY", "Recording file is not ready for download.", nil)
	case errors.Is(err, recording.ErrValidation):
		writeAPIError(r, http.StatusBadRequest, "VALIDATION_FAILED", "Recording request is invalid.", nil)
	default:
		writeAPIError(r, http.StatusInternalServerError, "RECORDING_OPERATION_FAILED", "Recording operation failed.", nil)
	}
}

func protectedMediaPath(relativePath string) string {
	parts := strings.Split(filepath.ToSlash(relativePath), "/")
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
	}
	return "/_protected_media/" + strings.Join(parts, "/")
}

func contentDisposition(name string) string {
	fallback := strings.Map(func(r rune) rune {
		if r >= 0x20 && r <= 0x7e && r != '"' && r != '\\' {
			return r
		}
		return -1
	}, name)
	if strings.TrimSpace(fallback) == "" {
		fallback = "recording"
	}
	return `attachment; filename="` + fallback + `"; filename*=UTF-8''` + url.PathEscape(name)
}

func recordingContentType(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".flv":
		return "video/x-flv"
	case ".mp4":
		return "video/mp4"
	case ".mkv":
		return "video/x-matroska"
	default:
		return "application/octet-stream"
	}
}
