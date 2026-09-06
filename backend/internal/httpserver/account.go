package httpserver

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/7grecorder/7grecorder/backend/internal/account"
	"github.com/7grecorder/7grecorder/backend/internal/config"
	"github.com/7grecorder/7grecorder/backend/internal/db"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
)

func bindAccountHandlers(cfg config.Config, s *ghttp.Server) {
	s.BindHandler("/api/v1/accounts", func(r *ghttp.Request) {
		switch r.Method {
		case http.MethodGet:
			withAccountStore(r, cfg, func(actor account.User, store account.Store) {
				items, err := store.List(r.Context(), actor)
				if err != nil {
					writeAccountError(r, err)
					return
				}
				r.Response.WriteJson(g.Map{"items": items, "page": 1, "page_size": len(items), "total": len(items)})
			})
		case http.MethodPost:
			withAccountStore(r, cfg, func(actor account.User, store account.Store) {
				var req account.CreateRequest
				if err := json.Unmarshal(r.GetBody(), &req); err != nil {
					writeAPIError(r, http.StatusBadRequest, "BAD_REQUEST", "Invalid JSON request body.", nil)
					return
				}
				created, err := store.Create(r.Context(), actor, req)
				if err != nil {
					writeAccountError(r, err)
					return
				}
				r.Response.Status = http.StatusCreated
				r.Response.WriteJson(created)
			})
		default:
			requireMethod(r, http.MethodGet)
		}
	})

	s.BindHandler("/api/v1/accounts/{id}", func(r *ghttp.Request) {
		id := r.Get("id").Int64()
		switch r.Method {
		case http.MethodGet:
			withAccountStore(r, cfg, func(actor account.User, store account.Store) {
				item, err := store.Get(r.Context(), actor, id)
				if err != nil {
					writeAccountError(r, err)
					return
				}
				r.Response.WriteJson(item)
			})
		case http.MethodPatch:
			withAccountStore(r, cfg, func(actor account.User, store account.Store) {
				var req account.UpdateRequest
				if err := json.Unmarshal(r.GetBody(), &req); err != nil {
					writeAPIError(r, http.StatusBadRequest, "BAD_REQUEST", "Invalid JSON request body.", nil)
					return
				}
				updated, err := store.Update(r.Context(), actor, id, req)
				if err != nil {
					writeAccountError(r, err)
					return
				}
				r.Response.WriteJson(updated)
			})
		default:
			requireMethod(r, http.MethodGet)
		}
	})

	s.BindHandler("/api/v1/accounts/{id}/policy", func(r *ghttp.Request) {
		if !requireMethod(r, http.MethodPut) {
			return
		}
		id := r.Get("id").Int64()
		withAccountStore(r, cfg, func(actor account.User, store account.Store) {
			var req account.PolicyUpsert
			if err := json.Unmarshal(r.GetBody(), &req); err != nil {
				writeAPIError(r, http.StatusBadRequest, "BAD_REQUEST", "Invalid JSON request body.", nil)
				return
			}
			policy, err := store.UpsertPolicy(r.Context(), actor, id, req)
			if err != nil {
				writeAccountError(r, err)
				return
			}
			r.Response.WriteJson(policy)
		})
	})
}

func withAccountStore(r *ghttp.Request, cfg config.Config, fn func(account.User, account.Store)) {
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
	fn(actor, account.NewStore(database))
}

func writeAccountError(r *ghttp.Request, err error) {
	switch {
	case errors.Is(err, account.ErrForbidden):
		writeAPIError(r, http.StatusForbidden, "FORBIDDEN", "Account operation is not allowed.", nil)
	case errors.Is(err, account.ErrNotFound):
		writeAPIError(r, http.StatusNotFound, "ACCOUNT_NOT_FOUND", "Account was not found.", nil)
	case errors.Is(err, account.ErrValidation):
		writeAPIError(r, http.StatusBadRequest, "VALIDATION_FAILED", "Account request is invalid.", nil)
	case errors.Is(err, account.ErrUsernameInUse):
		writeAPIError(r, http.StatusConflict, "USERNAME_IN_USE", "Username is already in use.", nil)
	case errors.Is(err, account.ErrCannotDisableSelf):
		writeAPIError(r, http.StatusConflict, "CANNOT_DISABLE_SELF", "The current account cannot disable itself.", nil)
	default:
		writeAPIError(r, http.StatusInternalServerError, "ACCOUNT_OPERATION_FAILED", "Account operation failed.", nil)
	}
}
