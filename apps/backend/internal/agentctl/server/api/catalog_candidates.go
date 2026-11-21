package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/common/sessioncatalog"
	"go.uber.org/zap"
)

const catalogCandidatesRequestLimit = 4 << 10

// catalogCandidatesRoute is TaskProbePath relative to the /api/v1 group.
var catalogCandidatesRoute = strings.TrimPrefix(sessioncatalog.TaskProbePath, "/api/v1")

// handleCatalogCandidatesDiscover is backend-only: it requires the current
// control credential and the exact instance identity, and it accepts only the
// expected owner. No launch input, command or catalog is accepted or returned.
func (s *Server) handleCatalogCandidatesDiscover(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if !c.GetBool(authenticatedControlContextKey) {
		c.JSON(http.StatusForbidden, gin.H{errKey: "authenticated control is required"})
		return
	}
	instanceID := ""
	if s.cfg != nil {
		instanceID = s.cfg.InstanceID
	}
	if instanceID == "" || strings.TrimSpace(c.GetHeader(InstanceIDHeader)) != instanceID {
		c.JSON(http.StatusForbidden, gin.H{errKey: "instance ownership is required"})
		return
	}
	var request sessioncatalog.TaskProbeRequest
	decoder := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, catalogCandidatesRequestLimit))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil || decoder.More() || request.Validate() != nil {
		c.JSON(http.StatusBadRequest, gin.H{errKey: "invalid catalog probe request"})
		return
	}
	reply, err := s.procMgr.DiscoverCatalogCandidates(c.Request.Context(), instanceID, request)
	if err != nil {
		status := catalogCandidatesErrorStatus(err)
		s.logger.Info("task catalog probe refused", zap.Int("status", status), zap.String("request_id", request.RequestID))
		c.JSON(status, gin.H{errKey: "task catalog probe is unavailable"})
		return
	}
	s.logger.Info("task catalog probe completed",
		zap.String("request_id", request.RequestID),
		zap.String("outcome", reply.Result.Outcome),
		zap.String("capability", reply.Capability))
	c.JSON(http.StatusOK, reply)
}

func catalogCandidatesErrorStatus(err error) int {
	switch {
	case errors.Is(err, sessioncatalog.ErrTaskProbeRequest):
		return http.StatusBadRequest
	case errors.Is(err, sessioncatalog.ErrTaskProbeOwner):
		return http.StatusConflict
	case errors.Is(err, process.ErrCatalogProbeBusy):
		return http.StatusTooManyRequests
	default:
		return http.StatusServiceUnavailable
	}
}
