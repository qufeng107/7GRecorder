package httpserver

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/7grecorder/7grecorder/backend/internal/account"
	"github.com/7grecorder/7grecorder/backend/internal/config"
	"github.com/7grecorder/7grecorder/backend/internal/db"
	"github.com/7grecorder/7grecorder/backend/internal/upload"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
)

func bindUploadHandlers(cfg config.Config, s *ghttp.Server) {
	s.BindHandler("/api/v1/credentials", func(r *ghttp.Request) {
		switch r.Method {
		case http.MethodGet:
			withUploadStore(r, cfg, func(actor account.User, store upload.Store) {
				items, err := store.ListCredentials(r.Context(), actor)
				if err != nil {
					writeUploadError(r, err)
					return
				}
				r.Response.WriteJson(g.Map{"items": items, "page": 1, "page_size": len(items), "total": len(items)})
			})
		case http.MethodPost:
			withUploadStore(r, cfg, func(actor account.User, store upload.Store) {
				var req upload.CredentialCreate
				if err := json.Unmarshal(r.GetBody(), &req); err != nil {
					writeAPIError(r, http.StatusBadRequest, "BAD_REQUEST", "Invalid JSON request body.", nil)
					return
				}
				created, err := store.CreateCredential(r.Context(), actor, req)
				if err != nil {
					writeUploadError(r, err)
					return
				}
				r.Response.Status = http.StatusCreated
				r.Response.WriteJson(created)
			})
		default:
			requireMethod(r, http.MethodGet)
		}
	})

	s.BindHandler("/api/v1/recording-profiles/{id}/publishing/bilibili", func(r *ghttp.Request) {
		id := r.Get("id").Int64()
		switch r.Method {
		case http.MethodGet:
			withUploadStore(r, cfg, func(actor account.User, store upload.Store) {
				item, err := store.GetBilibiliConfig(r.Context(), actor, id)
				if err != nil {
					writeUploadError(r, err)
					return
				}
				r.Response.WriteJson(item)
			})
		case http.MethodPut:
			withUploadStore(r, cfg, func(actor account.User, store upload.Store) {
				var req upload.PublishingConfigUpsert
				if err := json.Unmarshal(r.GetBody(), &req); err != nil {
					writeAPIError(r, http.StatusBadRequest, "BAD_REQUEST", "Invalid JSON request body.", nil)
					return
				}
				item, err := store.UpsertBilibiliConfig(r.Context(), actor, id, req)
				if err != nil {
					writeUploadError(r, err)
					return
				}
				r.Response.WriteJson(item)
			})
		default:
			requireMethod(r, http.MethodGet)
		}
	})

	s.BindHandler("/api/v1/recording-profiles/{id}/storage/cos", func(r *ghttp.Request) {
		id := r.Get("id").Int64()
		switch r.Method {
		case http.MethodGet:
			withUploadStore(r, cfg, func(actor account.User, store upload.Store) {
				item, err := store.GetCOSConfig(r.Context(), actor, id)
				if err != nil {
					writeUploadError(r, err)
					return
				}
				r.Response.WriteJson(item)
			})
		case http.MethodPut:
			withUploadStore(r, cfg, func(actor account.User, store upload.Store) {
				var req upload.COSConfigUpsert
				if err := json.Unmarshal(r.GetBody(), &req); err != nil {
					writeAPIError(r, http.StatusBadRequest, "BAD_REQUEST", "Invalid JSON request body.", nil)
					return
				}
				item, err := store.UpsertCOSConfig(r.Context(), actor, id, req)
				if err != nil {
					writeUploadError(r, err)
					return
				}
				r.Response.WriteJson(item)
			})
		default:
			requireMethod(r, http.MethodGet)
		}
	})

	s.BindHandler("/api/v1/upload-modules/actions/reconcile", func(r *ghttp.Request) {
		if !requireMethod(r, http.MethodPost) {
			return
		}
		withUploadStore(r, cfg, func(actor account.User, store upload.Store) {
			result, err := store.Reconcile(r.Context(), actor)
			if err != nil {
				writeUploadError(r, err)
				return
			}
			r.Response.WriteJson(result)
		})
	})
}

func withUploadStore(r *ghttp.Request, cfg config.Config, fn func(account.User, upload.Store)) {
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
	fn(actor, upload.NewStore(database, cfg))
}

func writeUploadError(r *ghttp.Request, err error) {
	switch {
	case errors.Is(err, upload.ErrForbidden):
		writeAPIError(r, http.StatusForbidden, "FORBIDDEN", "Upload module operation is not allowed.", nil)
	case errors.Is(err, upload.ErrNotFound):
		writeAPIError(r, http.StatusNotFound, "UPLOAD_RESOURCE_NOT_FOUND", "Upload module resource was not found.", nil)
	case errors.Is(err, upload.ErrValidation):
		writeAPIError(r, http.StatusBadRequest, "VALIDATION_FAILED", "Upload module request is invalid.", nil)
	default:
		writeAPIError(r, http.StatusInternalServerError, "UPLOAD_OPERATION_FAILED", "Upload module operation failed.", nil)
	}
}
