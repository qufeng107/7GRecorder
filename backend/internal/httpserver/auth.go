package httpserver

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/7grecorder/7grecorder/backend/internal/account"
	"github.com/7grecorder/7grecorder/backend/internal/config"
	"github.com/7grecorder/7grecorder/backend/internal/db"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
)

const sessionCookieName = "7gr_session"

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func bindAuthHandlers(cfg config.Config, s *ghttp.Server) {
	s.BindHandler("/api/v1/auth/login", func(r *ghttp.Request) {
		if !requireMethod(r, http.MethodPost) {
			return
		}
		var req loginRequest
		if err := json.Unmarshal(r.GetBody(), &req); err != nil {
			writeAPIError(r, http.StatusBadRequest, "BAD_REQUEST", "Invalid JSON request body.", nil)
			return
		}
		if strings.TrimSpace(req.Username) == "" || req.Password == "" {
			writeAPIError(r, http.StatusBadRequest, "VALIDATION_FAILED", "Username and password are required.", nil)
			return
		}

		database, err := db.Open(r.Context(), cfg)
		if err != nil {
			writeAPIError(r, http.StatusInternalServerError, "DATABASE_UNAVAILABLE", "Database is unavailable.", nil)
			return
		}
		defer database.Close()

		user, token, expiresAt, err := account.NewStore(database).Login(r.Context(), req.Username, req.Password, 24*time.Hour)
		if errors.Is(err, account.ErrInvalidCredentials) || errors.Is(err, account.ErrDisabledUser) {
			writeAPIError(r, http.StatusUnauthorized, "INVALID_CREDENTIALS", "Invalid username or password.", nil)
			return
		}
		if err != nil {
			writeAPIError(r, http.StatusInternalServerError, "LOGIN_FAILED", "Login failed.", nil)
			return
		}
		r.Cookie.SetHttpCookie(&http.Cookie{
			Name:     sessionCookieName,
			Value:    token,
			Path:     "/",
			Expires:  expiresAt,
			MaxAge:   int(time.Until(expiresAt).Seconds()),
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
		})
		r.Response.WriteJson(g.Map{
			"user":       user,
			"expires_at": expiresAt.Format(time.RFC3339),
		})
	})

	s.BindHandler("/api/v1/auth/logout", func(r *ghttp.Request) {
		if !requireMethod(r, http.MethodPost) {
			return
		}
		token := r.Cookie.Get(sessionCookieName)
		database, err := db.Open(r.Context(), cfg)
		if err != nil {
			writeAPIError(r, http.StatusInternalServerError, "DATABASE_UNAVAILABLE", "Database is unavailable.", nil)
			return
		}
		defer database.Close()
		if err := account.NewStore(database).Logout(r.Context(), token); err != nil {
			writeAPIError(r, http.StatusInternalServerError, "LOGOUT_FAILED", "Logout failed.", nil)
			return
		}
		r.Cookie.SetHttpCookie(&http.Cookie{
			Name:     sessionCookieName,
			Value:    "",
			Path:     "/",
			MaxAge:   -1,
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
		})
		r.Response.WriteJson(g.Map{"status": "ok"})
	})

	s.BindHandler("/api/v1/me", func(r *ghttp.Request) {
		if !requireMethod(r, http.MethodGet) {
			return
		}
		token := r.Cookie.Get(sessionCookieName)
		database, err := db.Open(r.Context(), cfg)
		if err != nil {
			writeAPIError(r, http.StatusInternalServerError, "DATABASE_UNAVAILABLE", "Database is unavailable.", nil)
			return
		}
		defer database.Close()
		user, err := account.NewStore(database).UserBySessionToken(r.Context(), token)
		if errors.Is(err, account.ErrNotFound) || errors.Is(err, account.ErrDisabledUser) {
			writeAPIError(r, http.StatusUnauthorized, "NOT_AUTHENTICATED", "Login is required.", nil)
			return
		}
		if err != nil {
			writeAPIError(r, http.StatusInternalServerError, "SESSION_LOOKUP_FAILED", "Session lookup failed.", nil)
			return
		}
		r.Response.WriteJson(g.Map{"user": user})
	})
}

func requireMethod(r *ghttp.Request, method string) bool {
	if r.Method == method {
		return true
	}
	r.Response.Status = http.StatusMethodNotAllowed
	r.Response.Header().Set("Allow", method)
	r.Response.WriteJson(g.Map{
		"error": g.Map{
			"code":       "METHOD_NOT_ALLOWED",
			"message":    "Method not allowed.",
			"details":    nil,
			"request_id": requestID(r),
		},
	})
	return false
}

func writeAPIError(r *ghttp.Request, status int, code string, message string, details interface{}) {
	r.Response.Status = status
	r.Response.WriteJson(g.Map{
		"error": g.Map{
			"code":       code,
			"message":    message,
			"details":    details,
			"request_id": requestID(r),
		},
	})
}

func requestID(r *ghttp.Request) string {
	return r.GetCtxVar("RequestId").String()
}
