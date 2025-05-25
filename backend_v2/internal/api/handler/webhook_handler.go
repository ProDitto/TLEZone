package handler

import (
	"encoding/json"
	"log"
	"net/http"
	"tle_zone_v2/internal/app/service"
	"tle_zone_v2/internal/common"

	"github.com/go-chi/chi/v5"
)

type WebhookHandler struct {
	webhookService *service.WebhookService
}

func NewWebhookHandler(ws *service.WebhookService) *WebhookHandler {
	return &WebhookHandler{webhookService: ws}
}

func (h *WebhookHandler) RegisterRoutes(r chi.Router) {
	// This endpoint should be secured, e.g., with a shared secret in a header
	// or by checking the source IP of the executor service.
	r.Post("/execution", h.handleExecutionResult)
}

func (h *WebhookHandler) handleExecutionResult(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement webhook security (e.g., X-Webhook-Secret header check)
	// secret := r.Header.Get("X-Webhook-Secret")
	// if secret != config.AppConfig.WebhookSecret {
	//    common.RespondWithError(w, http.StatusUnauthorized, "Invalid webhook secret")
	//    return
	// }

	var payload service.ExecutionResultPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		log.Printf("ERROR: Webhook: Invalid payload: %v", err)
		common.RespondWithError(w, http.StatusBadRequest, "Invalid webhook payload")
		return
	}
	defer r.Body.Close()

	if err := h.webhookService.HandleExecutionResult(r.Context(), payload); err != nil {
		log.Printf("ERROR: Webhook: Error handling result for JobID %s: %v", payload.JobID, err)
		common.RespondWithError(w, common.HTTPStatusFromError(err), err.Error())
		return
	}
	common.RespondWithJSON(w, http.StatusOK, map[string]string{"message": "Webhook processed for job " + payload.JobID})
}
