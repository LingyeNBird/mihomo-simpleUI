package api

import (
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"mihomo-webui-proxy/backend/internal/service"
)

const sessionCookieName = "mihomo_session"

type Server struct {
	service *service.Service
	static  string
}

type subscriptionRequest struct {
	Name    string `json:"name" binding:"required"`
	URL     string `json:"url" binding:"required"`
	Enabled bool   `json:"enabled"`
}

type selectNodeRequest struct {
	NodeName string `json:"nodeName" binding:"required"`
}

type loginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type changePasswordRequest struct {
	CurrentPassword string `json:"currentPassword" binding:"required"`
	NewPassword     string `json:"newPassword" binding:"required"`
}

func NewRouter(svc *service.Service, staticDir string) *gin.Engine {
	server := &Server{service: svc, static: staticDir}
	router := gin.Default()

	router.GET("/api/health", server.health)
	router.GET("/api/auth/status", server.authStatus)
	router.POST("/api/auth/login", server.login)
	router.POST("/api/auth/logout", server.logout)
	router.POST("/api/auth/change-password", server.changePassword)
	router.GET("/", server.index)

	authed := router.Group("/")
	authed.Use(server.requireSession(false))
	authed.GET("/api/status", server.status)
	authed.GET("/api/subscriptions", server.listSubscriptions)
	authed.GET("/api/subscriptions/:id/content", server.subscriptionContent)
	authed.POST("/api/subscriptions", server.createSubscription)
	authed.PUT("/api/subscriptions/:id", server.updateSubscription)
	authed.DELETE("/api/subscriptions/:id", server.deleteSubscription)
	authed.POST("/api/subscriptions/:id/refresh", server.refreshSubscription)
	authed.POST("/api/config/sync", server.syncConfig)
	authed.GET("/api/proxy-groups", server.proxyGroups)
	authed.POST("/api/proxy-groups/:group/select", server.selectNode)

	passwordOnly := router.Group("/")
	passwordOnly.Use(server.requireSession(true))
	passwordOnly.GET("/assets/*filepath", server.serveAuthedAsset)

	router.NoRoute(server.noRoute)

	return router
}

func (s *Server) requireSession(allowPasswordChangeOnly bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		status := s.service.AuthStatus(c.Request.Context(), sessionIDFromContext(c))
		if !status.Authenticated {
			if strings.HasPrefix(c.Request.URL.Path, "/api/") {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			} else {
				c.Redirect(http.StatusFound, "/")
			}
			c.Abort()
			return
		}
		if status.MustChangePassword && !allowPasswordChangeOnly {
			c.JSON(http.StatusForbidden, gin.H{"error": "password change required", "mustChangePassword": true})
			c.Abort()
			return
		}
		c.Set("authStatus", status)
		c.Next()
	}
}

func (s *Server) health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (s *Server) authStatus(c *gin.Context) {
	c.JSON(http.StatusOK, s.service.AuthStatus(c.Request.Context(), sessionIDFromContext(c)))
}

func (s *Server) login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	status, session, err := s.service.Login(c.Request.Context(), req.Username, req.Password)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}
	setSessionCookie(c, session.ID)
	c.JSON(http.StatusOK, status)
}

func (s *Server) logout(c *gin.Context) {
	_ = s.service.Logout(c.Request.Context(), sessionIDFromContext(c))
	clearSessionCookie(c)
	c.Status(http.StatusNoContent)
}

func (s *Server) changePassword(c *gin.Context) {
	var req changePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	status, err := s.service.ChangePassword(c.Request.Context(), sessionIDFromContext(c), req.CurrentPassword, req.NewPassword)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, status)
}

func (s *Server) status(c *gin.Context) {
	c.JSON(http.StatusOK, s.service.GetStatus(c.Request.Context()))
}

func (s *Server) listSubscriptions(c *gin.Context) {
	items, err := s.service.ListSubscriptions(c.Request.Context())
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, items)
}

func (s *Server) createSubscription(c *gin.Context) {
	var req subscriptionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	result, err := s.service.CreateSubscription(c.Request.Context(), req.Name, req.URL, req.Enabled)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusCreated, result)
}

func (s *Server) subscriptionContent(c *gin.Context) {
	id, err := parseID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	result, err := s.service.GetSubscriptionContent(c.Request.Context(), id)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func (s *Server) updateSubscription(c *gin.Context) {
	id, err := parseID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var req subscriptionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	item, err := s.service.UpdateSubscription(c.Request.Context(), id, req.Name, req.URL, req.Enabled)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, item)
}

func (s *Server) deleteSubscription(c *gin.Context) {
	id, err := parseID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := s.service.DeleteSubscription(c.Request.Context(), id); err != nil {
		writeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (s *Server) refreshSubscription(c *gin.Context) {
	id, err := parseID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	result, err := s.service.RefreshSubscription(c.Request.Context(), id)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func (s *Server) syncConfig(c *gin.Context) {
	result, err := s.service.SyncConfig(c.Request.Context())
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func (s *Server) proxyGroups(c *gin.Context) {
	items, err := s.service.GetProxyGroups(c.Request.Context())
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, items)
}

func (s *Server) selectNode(c *gin.Context) {
	var req selectNodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := s.service.SelectNode(c.Request.Context(), c.Param("group"), req.NodeName); err != nil {
		writeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (s *Server) index(c *gin.Context) {
	status := s.service.AuthStatus(c.Request.Context(), sessionIDFromContext(c))
	if !status.Authenticated {
		c.Header("Content-Type", "text/html; charset=utf-8")
		c.String(http.StatusOK, renderShell(false, false))
		return
	}
	if status.MustChangePassword {
		c.Header("Content-Type", "text/html; charset=utf-8")
		c.String(http.StatusOK, renderShell(true, true))
		return
	}
	c.File(filepath.Join(s.static, "index.html"))
}

func (s *Server) serveAuthedAsset(c *gin.Context) {
	asset := filepath.Clean(strings.TrimPrefix(c.Param("filepath"), "/"))
	if asset == "." || asset == "" {
		c.Status(http.StatusNotFound)
		return
	}
	c.File(filepath.Join(s.static, "assets", asset))
}

func (s *Server) noRoute(c *gin.Context) {
	if strings.HasPrefix(c.Request.URL.Path, "/api/") {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	status := s.service.AuthStatus(c.Request.Context(), sessionIDFromContext(c))
	if !status.Authenticated {
		c.Header("Content-Type", "text/html; charset=utf-8")
		c.String(http.StatusOK, renderShell(false, false))
		return
	}
	if status.MustChangePassword {
		c.Header("Content-Type", "text/html; charset=utf-8")
		c.String(http.StatusOK, renderShell(true, true))
		return
	}
	if filepath.Ext(c.Request.URL.Path) != "" {
		c.Status(http.StatusNotFound)
		return
	}
	c.File(filepath.Join(s.static, "index.html"))
}

func parseID(raw string) (int64, error) {
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, errors.New("invalid id")
	}
	return id, nil
}

func writeError(c *gin.Context, err error) {
	c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
}

func sessionIDFromContext(c *gin.Context) string {
	value, _ := c.Cookie(sessionCookieName)
	return value
}

func setSessionCookie(c *gin.Context, sessionID string) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(sessionCookieName, sessionID, 7*24*3600, "/", "", false, true)
}

func clearSessionCookie(c *gin.Context) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(sessionCookieName, "", -1, "/", "", false, true)
}

func renderShell(authenticated bool, mustChangePassword bool) string {
	mode := "login"
	if authenticated && mustChangePassword {
		mode = "password"
	}
	return fmt.Sprintf(`<!doctype html>
<html lang="zh-CN">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <title>Mihomo WebUI Proxy</title>
    <style>
      :root { color-scheme: light; }
      * { box-sizing: border-box; }
      body { margin: 0; font-family: Inter, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; background: linear-gradient(135deg, #0f172a 0%%, #0f766e 100%%); min-height: 100vh; display: flex; align-items: center; justify-content: center; padding: 24px; }
      .panel { width: 100%%; max-width: 420px; background: rgba(255,255,255,0.98); border-radius: 18px; box-shadow: 0 22px 60px rgba(15,23,42,0.28); padding: 28px; }
      h1 { margin: 0 0 8px; font-size: 28px; color: #0f172a; }
      p { margin: 0; color: #475569; line-height: 1.5; }
      form { display: grid; gap: 14px; margin-top: 24px; }
      label { display: grid; gap: 6px; font-size: 14px; color: #334155; }
      input { width: 100%%; border: 1px solid #cbd5e1; border-radius: 12px; padding: 12px 14px; font-size: 15px; }
      button { border: 0; border-radius: 12px; background: #0f766e; color: #fff; padding: 12px 16px; font-size: 15px; font-weight: 600; cursor: pointer; }
      button[disabled] { opacity: 0.7; cursor: wait; }
      .error { margin-top: 14px; color: #dc2626; font-size: 14px; min-height: 20px; }
      .meta { margin-top: 18px; font-size: 13px; color: #64748b; }
    </style>
  </head>
  <body>
    <div class="panel">
      <h1>Mihomo WebUI Proxy</h1>
      <p id="description"></p>
      <form id="auth-form">
        <label id="username-row">
          用户名
          <input id="username" name="username" autocomplete="username" value="mihomo" />
        </label>
        <label>
          当前密码
          <input id="password" name="password" type="password" autocomplete="current-password" />
        </label>
        <label id="next-password-row" style="display:none;">
          新密码
          <input id="newPassword" name="newPassword" type="password" autocomplete="new-password" />
        </label>
        <button id="submit" type="submit"></button>
      </form>
      <div class="error" id="error"></div>
      <div class="meta">首次访问默认账号密码均为 <strong>mihomo</strong>。</div>
    </div>
    <script>
      const mode = %q;
      const form = document.getElementById('auth-form');
      const description = document.getElementById('description');
      const error = document.getElementById('error');
      const submit = document.getElementById('submit');
      const usernameRow = document.getElementById('username-row');
      const nextPasswordRow = document.getElementById('next-password-row');
      const username = document.getElementById('username');
      const password = document.getElementById('password');
      const newPassword = document.getElementById('newPassword');
      if (mode === 'password') {
        description.textContent = '首次登录后必须修改密码，修改成功后将直接进入控制面板。';
        submit.textContent = '修改密码';
        usernameRow.style.display = 'none';
        nextPasswordRow.style.display = 'grid';
      } else {
        description.textContent = '请先登录后再访问控制面板内容。';
        submit.textContent = '登录';
      }
      form.addEventListener('submit', async (event) => {
        event.preventDefault();
        submit.disabled = true;
        error.textContent = '';
        try {
          const endpoint = mode === 'password' ? '/api/auth/change-password' : '/api/auth/login';
          const payload = mode === 'password'
            ? { currentPassword: password.value, newPassword: newPassword.value }
            : { username: username.value, password: password.value };
          const response = await fetch(endpoint, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            credentials: 'same-origin',
            body: JSON.stringify(payload),
          });
          const body = await response.json().catch(() => ({}));
          if (!response.ok) {
            throw new Error(body.error || '操作失败');
          }
          window.location.href = '/';
        } catch (err) {
          error.textContent = err instanceof Error ? err.message : String(err);
        } finally {
          submit.disabled = false;
        }
      });
    </script>
  </body>
</html>`, mode)
}
