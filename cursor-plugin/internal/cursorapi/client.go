package cursorapi

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"cursorplugin/internal/cursorproto"
)

const (
	maxModelResponseBytes    = 4 << 20
	defaultHeartbeatInterval = 5 * time.Second
	defaultFirstFrameTimeout = 30 * time.Second
	defaultClientVersion     = "cli-2026.07.08-0c04a8a"
)

var ErrFirstFrameTimeout = errors.New("Cursor transport timed out before first response")

type Config struct {
	BaseURL           string
	ClientVersion     string
	HTTPClient        *http.Client
	HeartbeatInterval time.Duration
	FirstFrameTimeout time.Duration
}

type Client struct {
	baseURL       *url.URL
	clientVersion string
	httpClient    *http.Client
	heartbeat     time.Duration
	firstFrame    time.Duration
}

func NewClient(config Config) (*Client, error) {
	baseURL, err := url.Parse(config.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse Cursor base URL: %w", err)
	}
	if baseURL.Scheme != "https" || baseURL.Host == "" {
		return nil, errors.New("Cursor base URL must be HTTPS")
	}
	client := config.HTTPClient
	if client == nil {
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.ForceAttemptHTTP2 = true
		client = &http.Client{Transport: transport}
	}
	if config.ClientVersion == "" {
		config.ClientVersion = defaultClientVersion
	}
	if config.HeartbeatInterval <= 0 {
		config.HeartbeatInterval = defaultHeartbeatInterval
	}
	if config.FirstFrameTimeout <= 0 {
		config.FirstFrameTimeout = defaultFirstFrameTimeout
	}
	return &Client{
		baseURL:       baseURL,
		clientVersion: config.ClientVersion,
		httpClient:    client,
		heartbeat:     config.HeartbeatInterval,
		firstFrame:    config.FirstFrameTimeout,
	}, nil
}

func (client *Client) DiscoverModels(ctx context.Context, accessToken string) ([]string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, client.endpoint("/agent.v1.AgentService/GetUsableModels"), http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("create Cursor model request: %w", err)
	}
	client.applyHeaders(request, requestHeaders{accessToken: accessToken, sessionID: randomUUID()}, "application/proto")
	response, err := client.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("request Cursor models: %w", err)
	}
	raw, readErr := io.ReadAll(io.LimitReader(response.Body, maxModelResponseBytes+1))
	closeErr := response.Body.Close()
	if err := errors.Join(readErr, closeErr); err != nil {
		return nil, fmt.Errorf("read Cursor models: %w", err)
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Cursor models returned HTTP %d", response.StatusCode)
	}
	if len(raw) > maxModelResponseBytes {
		return nil, errors.New("Cursor models response exceeds 4 MiB")
	}
	models, err := cursorproto.DecodeModels(raw)
	if err != nil {
		return nil, err
	}
	if len(models) == 0 {
		return nil, errors.New("Cursor returned no usable models")
	}
	return models, nil
}

type requestHeaders struct {
	accessToken string
	requestID   string
	sessionID   string
}

func (client *Client) applyHeaders(request *http.Request, values requestHeaders, contentType string) {
	request.Header.Set("content-type", contentType)
	request.Header.Set("connect-protocol-version", "1")
	request.Header.Set("te", "trailers")
	request.Header.Set("authorization", "Bearer "+values.accessToken)
	request.Header.Set("x-ghost-mode", "true")
	request.Header.Set("x-cursor-client-version", client.clientVersion)
	request.Header.Set("x-cursor-client-type", "cli")
	request.Header.Set("x-session-id", values.sessionID)
	if values.requestID != "" {
		request.Header.Set("x-request-id", values.requestID)
	}
}

func (client *Client) endpoint(path string) string {
	return strings.TrimRight(client.baseURL.String(), "/") + path
}

func randomUUID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		now := uint64(time.Now().UnixNano())
		binary.BigEndian.PutUint64(value[:8], now)
		binary.BigEndian.PutUint64(value[8:], now^0xa5a5a5a5a5a5a5a5)
	}
	value[6] = value[6]&0x0f | 0x40
	value[8] = value[8]&0x3f | 0x80
	return fmt.Sprintf(
		"%08x-%04x-%04x-%04x-%012x",
		value[0:4],
		value[4:6],
		value[6:8],
		value[8:10],
		value[10:16],
	)
}
