package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/agent/settings/controller"
)

func (h *Handlers) httpRefreshProfileModelCatalog(c *gin.Context) {
	// The forced probe and the session fan-out can outlast the server-wide
	// write timeout; the response carries both outcomes, so it must survive.
	// A writer without deadline support keeps the server default.
	_ = http.NewResponseController(c.Writer).SetWriteDeadline(time.Now().Add(controller.ProfileCatalogRefreshResponseBudget))
	if c.Request.ContentLength != 0 {
		c.JSON(http.StatusBadRequest, gin.H{errorResponseKey: "saved catalog refresh accepts no launch overrides"})
		return
	}
	scope, ok := requireProfileDiscoveryScope(c)
	if !ok {
		return
	}
	response, err := h.controller.RefreshProfileModelCatalog(c.Request.Context(), c.Param("id"), scope)
	if err != nil {
		writeProfileDiscoveryError(c, h.logger, "failed to refresh saved profile catalog", err)
		return
	}
	c.JSON(http.StatusOK, response)
}
