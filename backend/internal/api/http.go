package api

import (
	"errors"
	"net/http"
	"path/filepath"
	"strconv"

	"github.com/gin-gonic/gin"

	"mihomo-webui-proxy/backend/internal/service"
)

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

func NewRouter(svc *service.Service, staticDir string) *gin.Engine {
	server := &Server{service: svc, static: staticDir}
	router := gin.Default()
	router.GET("/api/health", server.health)
	router.GET("/api/status", server.status)
	router.GET("/api/subscriptions", server.listSubscriptions)
	router.GET("/api/subscriptions/:id/content", server.subscriptionContent)
	router.POST("/api/subscriptions", server.createSubscription)
	router.PUT("/api/subscriptions/:id", server.updateSubscription)
	router.DELETE("/api/subscriptions/:id", server.deleteSubscription)
	router.POST("/api/subscriptions/:id/refresh", server.refreshSubscription)
	router.POST("/api/config/sync", server.syncConfig)
	router.GET("/api/proxy-groups", server.proxyGroups)
	router.POST("/api/proxy-groups/:group/select", server.selectNode)

	indexFile := filepath.Join(staticDir, "index.html")
	router.StaticFS("/assets", gin.Dir(filepath.Join(staticDir, "assets"), false))
	router.NoRoute(func(c *gin.Context) {
		if filepath.Ext(c.Request.URL.Path) != "" {
			c.Status(http.StatusNotFound)
			return
		}
		c.File(indexFile)
	})
	return router
}

func (s *Server) health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"ok": true})
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
