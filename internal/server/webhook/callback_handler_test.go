package webhook_test

import (
	"bufio"
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/tonglil/buflogr"
	"github.com/weaveworks/tf-controller/internal/server/webhook"
)

const webhookSecret = "33713b74ae24b9be98a5c0f1dc4c864189294228"

func TestCallbackHandler_InvalidRequestMethod(t *testing.T) {
	handler := webhook.NewCallbackHandler(logr.Discard())
	testCases := []string{
		http.MethodGet,
		http.MethodPut,
		http.MethodHead,
		http.MethodPatch,
	}

	for _, method := range testCases {
		t.Run(method, func(t *testing.T) {
			request := httptest.NewRequest(method, "/callback?provider=github", nil)
			responseRecorder := httptest.NewRecorder()

			handler.ServeHTTP(responseRecorder, request)
			assert.Equal(t, http.StatusMethodNotAllowed, responseRecorder.Code)
		})
	}
}

func TestCallbackHandler_InvalidProvider(t *testing.T) {
	testCases := []struct {
		name        string
		url         string
		code        int
		logContains string
	}{
		{
			name:        "missing provider",
			url:         "/callback",
			code:        http.StatusBadRequest,
			logContains: "missing provider",
		},
		{
			name:        "invalid provider",
			url:         "/callback?provider=something",
			code:        http.StatusBadRequest,
			logContains: "Unsupported GIT_KIND value: something",
		},
	}

	var buf bytes.Buffer
	var log logr.Logger = buflogr.NewWithBuffer(&buf)

	handler := webhook.NewCallbackHandler(log)
	body, header, err := readRequestFixture("pull_request_event")
	if err != nil {
		t.Fatal(err)
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, testCase.url, bytes.NewReader(body))
			request.Header = header
			responseRecorder := httptest.NewRecorder()

			handler.ServeHTTP(responseRecorder, request)
			assert.Equal(t, testCase.code, responseRecorder.Code)
			assert.Contains(t, buf.String(), testCase.logContains)
		})
	}
}

func TestCallbackHandler_MissingHMAC(t *testing.T) {
	os.Setenv("WEBHOOK_HMAC_KEY", webhookSecret)

	var buf bytes.Buffer
	var log logr.Logger = buflogr.NewWithBuffer(&buf)

	handler := webhook.NewCallbackHandler(log)
	body, header, err := readRequestFixture("pull_request_event")
	if err != nil {
		t.Fatal(err)
	}

	header.Del(webhook.HubSignatureSHA256Header)
	header.Del(webhook.HubSignatureSHA1Header)

	request := httptest.NewRequest(http.MethodPost, "/callback?provider=github", bytes.NewReader(body))
	request.Header = header
	responseRecorder := httptest.NewRecorder()

	handler.ServeHTTP(responseRecorder, request)
	assert.Equal(t, http.StatusUnauthorized, responseRecorder.Code)
	assert.Contains(t, buf.String(), "missing hmac signature")
}

func TestCallbackHandler_InvalidHMAC(t *testing.T) {
	os.Setenv("WEBHOOK_HMAC_KEY", "invalid-key")

	var buf bytes.Buffer
	var log logr.Logger = buflogr.NewWithBuffer(&buf)

	handler := webhook.NewCallbackHandler(log)
	body, header, err := readRequestFixture("pull_request_event")
	if err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodPost, "/callback?provider=github", bytes.NewReader(body))
	request.Header = header
	responseRecorder := httptest.NewRecorder()

	handler.ServeHTTP(responseRecorder, request)
	assert.Equal(t, http.StatusBadRequest, responseRecorder.Code)
	assert.Contains(t, buf.String(), "invalid webhook signature parsing webhook request")
}

func TestCallbackHandler_PullRequestEvent(t *testing.T) {
	os.Setenv("WEBHOOK_HMAC_KEY", webhookSecret)

	var buf bytes.Buffer
	var log logr.Logger = buflogr.NewWithBuffer(&buf)

	handler := webhook.NewCallbackHandler(log)
	body, header, err := readRequestFixture("pull_request_event")
	if err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodPost, "/callback?provider=github", bytes.NewReader(body))
	request.Header = header
	responseRecorder := httptest.NewRecorder()

	handler.ServeHTTP(responseRecorder, request)
	assert.Equal(t, http.StatusAccepted, responseRecorder.Code)
	assert.Contains(t, buf.String(), "incoming hmac request is valid kind pull_request")
}

func TestCallbackHandler_CommentEvent(t *testing.T) {
	os.Setenv("WEBHOOK_HMAC_KEY", webhookSecret)

	var buf bytes.Buffer
	var log logr.Logger = buflogr.NewWithBuffer(&buf)

	handler := webhook.NewCallbackHandler(log)
	body, header, err := readRequestFixture("comment_event")
	if err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodPost, "/callback?provider=github", bytes.NewReader(body))
	request.Header = header
	responseRecorder := httptest.NewRecorder()

	handler.ServeHTTP(responseRecorder, request)
	assert.Equal(t, http.StatusAccepted, responseRecorder.Code)
	assert.Contains(t, buf.String(), "incoming hmac request is valid kind issue_comment")
}

func readFixture(filename string) ([]byte, error) {
	return os.ReadFile(fmt.Sprintf("fixtures/%s", filename))
}

func readRequestFixture(name string) ([]byte, http.Header, error) {
	requestContent, err := readFixture(fmt.Sprintf("%s.json", name))
	if err != nil {
		return []byte{}, http.Header{}, err
	}

	if requestContent[len(requestContent)-1] == '\n' {
		requestContent = requestContent[:len(requestContent)-1]
	}

	headerContent, err := readFixture(fmt.Sprintf("%s.header", name))
	if err != nil {
		return requestContent, http.Header{}, err
	}

	header := http.Header{}

	scanner := bufio.NewScanner(bytes.NewReader(headerContent))
	for scanner.Scan() {
		parts := strings.SplitN(scanner.Text(), ": ", 2)

		header.Add(parts[0], parts[1])
	}

	return requestContent, header, scanner.Err()
}
