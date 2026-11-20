package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func (h *Handlers) httpRefreshProfileModelCatalog(c *gin.Context) {
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
