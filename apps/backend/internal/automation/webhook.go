package automation

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/kandev/kandev/internal/common/logger"
)

const responseErrorKey = "error"

// WebhookHandler handles incoming webhook requests that fire automation triggers.
type WebhookHandler struct {
	svc    *Service
	logger *logger.Logger
}

// NewWebhookHandler creates a new webhook handler.

const maxWebhookBodyBytes = 1 << 20

func validateWebhookJSONPointer(pointer string) error {
	for _, part := range strings.Split(pointer[1:], "/") {
		for i := range len(part) {
			if part[i] != '~' {
				continue
			}
			if i+1 >= len(part) || (part[i+1] != '0' && part[i+1] != '1') {
				return errors.New("invalid webhook JSON pointer")
			}
		}
		if part == "-" || (len(part) > 1 && part[0] == '0' && allDigits(part)) {
			return errors.New("invalid webhook JSON pointer")
		}
		if len(part) > 1 && (part[0] == '+' || part[0] == '-') && allDigits(part[1:]) {
			return errors.New("invalid webhook JSON pointer")
		}
	}
	return nil
}

func allDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, char := range value {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}

func safeWebhookTriggerData(body []byte, pointers []string, triggerID, deliveryID string) (json.RawMessage, error) {
	projection := map[string]any{
		retryTriggerTypeKey: TriggerTypeWebhook,
		retryTriggerIDKey:   triggerID,
		"delivery_id":       deliveryID,
	}
	if len(pointers) > 32 {
		return nil, errors.New("too many webhook JSON pointers")
	}
	for _, pointer := range pointers {
		if len(pointer) == 0 || len(pointer) > 256 || !strings.HasPrefix(pointer, "/") {
			return nil, errors.New("invalid webhook JSON pointer")
		}
		if len(strings.Split(pointer[1:], "/")) > 8 {
			return nil, errors.New("webhook JSON pointer is too deep")
		}
		if err := validateWebhookJSONPointer(pointer); err != nil {
			return nil, err
		}
	}
	selected, err := projectWebhookPayload(body, pointers)
	if err != nil {
		return nil, err
	}
	if selected != nil {
		projection["payload"] = selected
	}
	encoded, err := json.Marshal(projection)
	return encoded, err
}

func projectWebhookPayload(body []byte, pointers []string) (map[string]json.RawMessage, error) {
	if len(body) == 0 {
		return nil, nil
	}
	if !json.Valid(body) {
		return nil, errors.New("invalid webhook JSON")
	}
	var document any
	if err := json.Unmarshal(body, &document); err != nil {
		return nil, errors.New("invalid webhook JSON")
	}
	selected := make(map[string]json.RawMessage, len(pointers))
	total := 0
	for _, pointer := range pointers {
		value, ok := webhookJSONPointer(document, strings.Split(pointer[1:], "/"))
		if !ok {
			continue
		}
		encoded, err := json.Marshal(value)
		if err != nil || len(encoded) > 8192 {
			return nil, errors.New("webhook JSON pointer value is too large")
		}
		total += len(encoded)
		if total > 64<<10 {
			return nil, errors.New("webhook JSON projection is too large")
		}
		selected[pointer] = encoded
	}
	return selected, nil
}

func webhookJSONPointer(value any, parts []string) (any, bool) {
	for _, part := range parts {
		part = strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")
		switch current := value.(type) {
		case map[string]any:
			var ok bool
			value, ok = current[part]
			if !ok {
				return nil, false
			}
		case []any:
			index, err := strconv.Atoi(part)
			if err != nil || index < 0 || index >= len(current) {
				return nil, false
			}
			value = current[index]
		default:
			return nil, false
		}
	}
	return value, true
}
func NewWebhookHandler(svc *Service, log *logger.Logger) *WebhookHandler {
	return &WebhookHandler{svc: svc, logger: log}
}

// Handle processes an incoming webhook POST request.
// URL format: POST /api/v1/automations/webhook/:id  with X-Webhook-Secret header
func (h *WebhookHandler) Handle(c *gin.Context) {
	automationID := c.Param("id")
	if automationID == "" {
		c.JSON(http.StatusBadRequest, gin.H{responseErrorKey: "automation id required"})
		return
	}

	a, err := h.svc.GetAutomation(c.Request.Context(), automationID)
	if err != nil || a == nil {
		c.JSON(http.StatusNotFound, gin.H{responseErrorKey: "automation not found"})
		return
	}
	if !a.Enabled {
		c.JSON(http.StatusConflict, gin.H{responseErrorKey: "automation is disabled"})
		return
	}

	// Secret must come via header — query params would leak into URLs/logs.
	// Compared in constant time so an attacker can't recover the secret byte
	// by byte from timing differences.
	secret := c.GetHeader("X-Webhook-Secret")
	if subtle.ConstantTimeCompare([]byte(secret), []byte(a.WebhookSecret)) != 1 {
		c.JSON(http.StatusUnauthorized, gin.H{responseErrorKey: "invalid webhook secret"})
		return
	}

	// A webhook endpoint has one unambiguous trigger identity.
	var webhookTrigger *AutomationTrigger
	for i := range a.Triggers {
		trigger := &a.Triggers[i]
		if trigger.Type == TriggerTypeWebhook && trigger.Enabled {
			if webhookTrigger != nil {
				c.JSON(http.StatusConflict, gin.H{responseErrorKey: "multiple enabled webhook triggers"})
				return
			}
			webhookTrigger = trigger
		}
	}
	if webhookTrigger == nil {
		c.JSON(http.StatusConflict, gin.H{responseErrorKey: "no enabled webhook trigger"})
		return
	}
	deliveryID := strings.TrimSpace(c.GetHeader("X-Kandev-Delivery-ID"))
	if deliveryID == "" || len(deliveryID) > 256 {
		c.JSON(http.StatusBadRequest, gin.H{responseErrorKey: "delivery id required"})
		return
	}
	var config WebhookTriggerConfig
	if len(webhookTrigger.Config) != 0 && json.Unmarshal(webhookTrigger.Config, &config) != nil {
		c.JSON(http.StatusConflict, gin.H{responseErrorKey: "invalid webhook trigger configuration"})
		return
	}
	body, readErr := io.ReadAll(io.LimitReader(c.Request.Body, maxWebhookBodyBytes+1))
	if readErr != nil {
		c.JSON(http.StatusBadRequest, gin.H{responseErrorKey: "failed to read body"})
		return
	}
	if len(body) > maxWebhookBodyBytes {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{responseErrorKey: "webhook body is too large"})
		return
	}
	triggerData, projectionErr := safeWebhookTriggerData(body, config.SafeJSONPointers, webhookTrigger.ID, deliveryID)
	if projectionErr != nil {
		c.JSON(http.StatusBadRequest, gin.H{responseErrorKey: projectionErr.Error()})
		return
	}
	dedupKey := "webhook:" + automationID + ":" + deliveryID
	result, fireErr := h.svc.FireTriggerWithInitialData(
		c.Request.Context(), automationID, webhookTrigger.ID, TriggerTypeWebhook,
		triggerData, body, dedupKey,
	)
	if fireErr != nil {
		h.logger.Error("failed to fire webhook trigger",
			zap.String("automation_id", automationID),
			zap.Error(fireErr))
		c.JSON(http.StatusInternalServerError, gin.H{responseErrorKey: "trigger failed"})
		return
	}
	if result.Skipped && !result.Duplicate {
		c.JSON(http.StatusConflict, gin.H{responseErrorKey: result.Reason})
		return
	}
	status := "triggered"
	if result.Duplicate {
		status = "duplicate"
	}
	c.JSON(http.StatusOK, gin.H{"status": status})
}
