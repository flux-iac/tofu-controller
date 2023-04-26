package webhook

import (
	"io"
	"net/http"
	"os"

	"github.com/fluxcd/pkg/runtime/logger"
	"github.com/go-logr/logr"
	"github.com/jenkins-x/go-scm/scm"
	"github.com/jenkins-x/go-scm/scm/factory"
)

const (
	HubSignatureSHA256Header = "X-Hub-Signature-256"
	HubSignatureSHA1Header   = "X-Hub-Signature"
)

type callbackHandler struct {
	log logr.Logger
}

func NewCallbackHandler(log logr.Logger) *callbackHandler {
	return &callbackHandler{
		log: log,
	}
}

func (h *callbackHandler) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		response.WriteHeader(http.StatusMethodNotAllowed)

		return
	}

	if request.Header.Get(HubSignatureSHA256Header) == "" {
		h.log.V(logger.DebugLevel).Info("missing hmac signature")
		response.WriteHeader(http.StatusUnauthorized)

		return
	}

	if request.URL.Query().Get("provider") == "" {
		h.log.V(logger.DebugLevel).Info("missing provider")
		response.WriteHeader(http.StatusBadRequest)

		return
	}

	scmClient, err := factory.NewWebHookService(request.URL.Query().Get("provider"))
	if err != nil {
		h.log.V(logger.DebugLevel).Error(err, "failed to create scm webhook client")
		response.WriteHeader(http.StatusBadRequest)

		return
	}

	hook, err := scmClient.Parse(request, func(_ scm.Webhook) (string, error) {
		return fetchHMACKey(), nil
	})
	if err != nil && (hook == nil || hook.Kind() != scm.WebhookKindPing) {
		h.log.V(logger.DebugLevel).Error(err, "parsing webhook request")
		response.WriteHeader(http.StatusBadRequest)

		return
	}

	log := h.log.WithValues(
		"kind", hook.Kind(),
		"organization", hook.Repository().Namespace,
		"repository", hook.Repository().Name,
	)

	log.V(logger.DebugLevel).Info("incoming hmac request is valid")

	switch hook.Kind() {
	case scm.WebhookKindPing:
		response.WriteHeader(http.StatusAccepted)

		return
	case scm.WebhookKindPullRequest:
		if err := handlePullRequest(log, hook); err != nil {
			log.V(logger.DebugLevel).Error(err, "processing pull request event")
			response.WriteHeader(http.StatusBadRequest)

			return
		}
	case scm.WebhookKindIssueComment:
		if err := handleComment(log, hook); err != nil {
			log.V(logger.DebugLevel).Error(err, "processing comment event")
			response.WriteHeader(http.StatusBadRequest)

			return
		}
	default:
		log.V(logger.DebugLevel).Info("unknown event type")
		response.WriteHeader(http.StatusBadRequest)

		return
	}

	response.WriteHeader(http.StatusAccepted)
	io.WriteString(response, "Webhook request is valid and processed")
}

func fetchHMACKey() string {
	return os.Getenv("WEBHOOK_HMAC_KEY")
}
