package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"sync"

	fhttp "github.com/bogdanfinn/fhttp"
	tlsclient "github.com/bogdanfinn/tls-client"
	"github.com/bogdanfinn/tls-client/profiles"
)

type claudeWebBrowserClientSlot struct {
	once   sync.Once
	client tlsclient.HttpClient
	err    error
}

// ClaudeWebBrowserTransport keeps Claude Web traffic on a Chrome browser TLS
// profile. Clients are isolated by account and proxy to avoid cross-account
// cookies, routes, and connection state.
type ClaudeWebBrowserTransport struct {
	clients sync.Map
}

func NewClaudeWebBrowserTransport() *ClaudeWebBrowserTransport {
	return &ClaudeWebBrowserTransport{}
}

func (t *ClaudeWebBrowserTransport) clientSlot(accountID int64, proxyURL string) *claudeWebBrowserClientSlot {
	key := strconv.FormatInt(accountID, 10) + "\x00" + proxyURL
	value, _ := t.clients.LoadOrStore(key, &claudeWebBrowserClientSlot{})
	return value.(*claudeWebBrowserClientSlot)
}

func (t *ClaudeWebBrowserTransport) Do(ctx context.Context, request *http.Request, proxyURL string, accountID int64) (*http.Response, error) {
	if t == nil {
		return nil, errors.New("Claude Web browser transport is nil")
	}
	if request == nil {
		return nil, errors.New("Claude Web request is nil")
	}
	slot := t.clientSlot(accountID, proxyURL)
	slot.once.Do(func() {
		slot.client, slot.err = newClaudeWebTLSClient(proxyURL)
	})
	if slot.err != nil {
		return nil, slot.err
	}

	fRequest, err := fhttp.NewRequestWithContext(ctx, request.Method, request.URL.String(), request.Body)
	if err != nil {
		return nil, errors.New("create Chrome transport request")
	}
	for key, values := range request.Header {
		for _, value := range values {
			fRequest.Header.Add(key, value)
		}
	}
	fRequest.Host = request.Host
	fRequest.ContentLength = request.ContentLength
	fRequest.Header[fhttp.HeaderOrderKey] = []string{
		"accept",
		"accept-language",
		"content-type",
		"anthropic-client-platform",
		"anthropic-device-id",
		"origin",
		"referer",
		"sec-ch-ua",
		"sec-ch-ua-mobile",
		"sec-ch-ua-platform",
		"sec-fetch-dest",
		"sec-fetch-mode",
		"sec-fetch-site",
		"user-agent",
		"cookie",
	}

	fResponse, err := slot.client.Do(fRequest)
	if err != nil {
		return nil, fmt.Errorf("Chrome transport request failed: %w", err)
	}
	header := make(http.Header, len(fResponse.Header))
	for key, values := range fResponse.Header {
		for _, value := range values {
			header.Add(key, value)
		}
	}
	return &http.Response{
		Status:        fResponse.Status,
		StatusCode:    fResponse.StatusCode,
		Proto:         fResponse.Proto,
		ProtoMajor:    fResponse.ProtoMajor,
		ProtoMinor:    fResponse.ProtoMinor,
		Header:        header,
		Body:          fResponse.Body,
		ContentLength: fResponse.ContentLength,
		Request:       request,
	}, nil
}

func newClaudeWebTLSClient(proxyURL string) (tlsclient.HttpClient, error) {
	options := []tlsclient.HttpClientOption{
		tlsclient.WithTimeoutSeconds(300),
		tlsclient.WithClientProfile(profiles.Chrome_146),
		tlsclient.WithCookieJar(tlsclient.NewCookieJar()),
		tlsclient.WithNotFollowRedirects(),
	}
	if proxyURL != "" {
		options = append(options, tlsclient.WithProxyUrl(proxyURL))
	}
	client, err := tlsclient.NewHttpClient(tlsclient.NewNoopLogger(), options...)
	if err != nil {
		return nil, fmt.Errorf("create Chrome transport client: %w", err)
	}
	return client, nil
}
