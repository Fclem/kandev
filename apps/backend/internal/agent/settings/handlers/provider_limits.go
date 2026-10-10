package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/settings/controller"
)

// responseErrorKey is the JSON field carrying an HTTP error message.
const responseErrorKey = "error"

func (h *Handlers) httpListProfileLimits(c *gin.Context) {
	limits, err := h.controller.ListProfileLimits(c.Request.Context())
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, controller.ErrProfileLimitStoreUnavailable) {
			status = http.StatusServiceUnavailable
		}
		h.logger.Warn("list provider limits failed", zap.Error(err))
		c.JSON(status, gin.H{responseErrorKey: "failed to list provider limits"})
		return
	}
	c.JSON(http.StatusOK, limits)
}
