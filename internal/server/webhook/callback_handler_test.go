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
	"github.com/onsi/gomega"
	"github.com/tonglil/buflogr"
	"github.com/weaveworks/tf-controller/internal/server/webhook"
)

const webhookSecret = "33713b74ae24b9be98a5c0f1dc4c864189294228"

func TestCallbackHandler_InvalidRequestMethod(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	handler := webhook.NewCallbackHandler(logr.Discard())
	testCases := []string{
		http.MethodGet,
		http.MethodPut,
		http.MethodHead,
		http.MethodPatch,
	}

	for _, method := range testCases {
		t.Run(method, func(_ *testing.T) {
			request := httptest.NewRequest(method, "/callback?provider=github", nil)
			responseRecorder := httptest.NewRecorder()

			handler.ServeHTTP(responseRecorder, request)
			g.Expect(responseRecorder.Code).To(gomega.Equal(http.StatusMethodNotAllowed))
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

	g := gomega.NewGomegaWithT(t)

	var (
		buf bytes.Buffer
		log logr.Logger = buflogr.NewWithBuffer(&buf)
	)

	handler := webhook.NewCallbackHandler(log)
	body, header, err := readRequestFixture("pull_request_event")
	g.Expect(err).NotTo(gomega.HaveOccurred())

	for _, testCase := range testCases {
		t.Run(testCase.name, func(_ *testing.T) {
			request := httptest.NewRequest(http.MethodPost, testCase.url, bytes.NewReader(body))
			request.Header = header
			responseRecorder := httptest.NewRecorder()

			handler.ServeHTTP(responseRecorder, request)
			g.Expect(responseRecorder.Code).To(gomega.Equal(testCase.code))
			g.Expect(buf.String()).To(gomega.ContainSubstring(testCase.logContains))
		})
	}
}

func TestCallbackHandler_MissingHMAC(t *testing.T) {
	os.Setenv("WEBHOOK_HMAC_KEY", webhookSecret)

	g := gomega.NewGomegaWithT(t)

	var (
		buf bytes.Buffer
		log logr.Logger = buflogr.NewWithBuffer(&buf)
	)

	handler := webhook.NewCallbackHandler(log)
	body, header, err := readRequestFixture("pull_request_event")
	g.Expect(err).NotTo(gomega.HaveOccurred())

	header.Del(webhook.HubSignatureSHA256Header)
	header.Del(webhook.HubSignatureSHA1Header)

	request := httptest.NewRequest(http.MethodPost, "/callback?provider=github", bytes.NewReader(body))
	request.Header = header
	responseRecorder := httptest.NewRecorder()

	handler.ServeHTTP(responseRecorder, request)
	g.Expect(responseRecorder.Code).To(gomega.Equal(http.StatusUnauthorized))
	g.Expect(buf.String()).To(gomega.ContainSubstring("missing hmac signature"))
}

func TestCallbackHandler_InvalidHMAC(t *testing.T) {
	os.Setenv("WEBHOOK_HMAC_KEY", "invalid-key")

	g := gomega.NewGomegaWithT(t)

	var (
		buf bytes.Buffer
		log logr.Logger = buflogr.NewWithBuffer(&buf)
	)

	handler := webhook.NewCallbackHandler(log)
	body, header, err := readRequestFixture("pull_request_event")
	g.Expect(err).NotTo(gomega.HaveOccurred())

	request := httptest.NewRequest(http.MethodPost, "/callback?provider=github", bytes.NewReader(body))
	request.Header = header
	responseRecorder := httptest.NewRecorder()

	handler.ServeHTTP(responseRecorder, request)
	g.Expect(responseRecorder.Code).To(gomega.Equal(http.StatusBadRequest))
	g.Expect(buf.String()).To(gomega.ContainSubstring("invalid webhook signature parsing webhook request"))
}

func TestCallbackHandler_PullRequestEvent(t *testing.T) {
	os.Setenv("WEBHOOK_HMAC_KEY", webhookSecret)

	g := gomega.NewGomegaWithT(t)

	var (
		buf bytes.Buffer
		log logr.Logger = buflogr.NewWithBuffer(&buf)
	)

	handler := webhook.NewCallbackHandler(log)
	body, header, err := readRequestFixture("pull_request_event")
	g.Expect(err).NotTo(gomega.HaveOccurred())

	request := httptest.NewRequest(http.MethodPost, "/callback?provider=github", bytes.NewReader(body))
	request.Header = header
	responseRecorder := httptest.NewRecorder()

	handler.ServeHTTP(responseRecorder, request)
	g.Expect(responseRecorder.Code).To(gomega.Equal(http.StatusAccepted))
	g.Expect(buf.String()).To(gomega.ContainSubstring("incoming hmac request is valid kind pull_request"))
}

func TestCallbackHandler_CommentEvent(t *testing.T) {
	os.Setenv("WEBHOOK_HMAC_KEY", webhookSecret)

	g := gomega.NewGomegaWithT(t)

	var (
		buf bytes.Buffer
		log logr.Logger = buflogr.NewWithBuffer(&buf)
	)

	handler := webhook.NewCallbackHandler(log)
	body, header, err := readRequestFixture("comment_event")
	g.Expect(err).NotTo(gomega.HaveOccurred())

	request := httptest.NewRequest(http.MethodPost, "/callback?provider=github", bytes.NewReader(body))
	request.Header = header
	responseRecorder := httptest.NewRecorder()

	handler.ServeHTTP(responseRecorder, request)
	g.Expect(responseRecorder.Code).To(gomega.Equal(http.StatusAccepted))
	g.Expect(buf.String()).To(gomega.ContainSubstring("incoming hmac request is valid kind issue_comment"))
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
