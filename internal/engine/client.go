package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/nlink-jp/voice-studio-mcp/internal/toolerr"
)

// errBodyHead limits how much of an error response body is surfaced in
// structured error details.
const errBodyHead = 512

// Client is a minimal HTTP client for the VOICEVOX-compatible engine API.
type Client struct {
	baseURL string
	http    *http.Client
}

// NewClient returns a client for baseURL (e.g. "http://127.0.0.1:10101").
// timeout bounds each request; /synthesis on long text can be slow, so pass
// the configured request timeout.
func NewClient(baseURL string, timeout time.Duration) *Client {
	return &Client{
		baseURL: baseURL,
		http:    &http.Client{Timeout: timeout},
	}
}

// BaseURL returns the engine base URL.
func (c *Client) BaseURL() string { return c.baseURL }

// Version returns the engine version string (GET /version).
func (c *Client) Version(ctx context.Context) (string, error) {
	body, err := c.do(ctx, http.MethodGet, "/version", nil, nil)
	if err != nil {
		return "", err
	}
	var v string
	if err := json.Unmarshal(body, &v); err != nil {
		return "", toolerr.Newf(toolerr.CodeEngineRequest, "parse /version response: %v", err)
	}
	return v, nil
}

// Speakers returns the installed speakers (GET /speakers).
func (c *Client) Speakers(ctx context.Context) ([]Speaker, error) {
	body, err := c.do(ctx, http.MethodGet, "/speakers", nil, nil)
	if err != nil {
		return nil, err
	}
	var out []Speaker
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, toolerr.Newf(toolerr.CodeEngineRequest, "parse /speakers response: %v", err)
	}
	return out, nil
}

// AivmModels returns the installed AIVM voice models keyed by model UUID
// (GET /aivm_models — AivisSpeech-specific; not part of the VOICEVOX API).
// Manifests carry author-declared license text; see ADR-0008.
func (c *Client) AivmModels(ctx context.Context) (map[string]AivmModel, error) {
	body, err := c.do(ctx, http.MethodGet, "/aivm_models", nil, nil)
	if err != nil {
		return nil, err
	}
	var out map[string]AivmModel
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, toolerr.Newf(toolerr.CodeEngineRequest, "parse /aivm_models response: %v", err)
	}
	return out, nil
}

// AudioQuery builds a synthesis query for text with the given style
// (POST /audio_query?text=...&speaker=<styleID>).
func (c *Client) AudioQuery(ctx context.Context, text string, styleID int) (AudioQuery, error) {
	q := url.Values{}
	q.Set("text", text)
	q.Set("speaker", strconv.Itoa(styleID))
	body, err := c.do(ctx, http.MethodPost, "/audio_query", q, nil)
	if err != nil {
		return nil, err
	}
	var out AudioQuery
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, toolerr.Newf(toolerr.CodeEngineRequest, "parse /audio_query response: %v", err)
	}
	return out, nil
}

// Synthesis renders query into WAV bytes (POST /synthesis?speaker=<styleID>).
func (c *Client) Synthesis(ctx context.Context, query AudioQuery, styleID int) ([]byte, error) {
	q := url.Values{}
	q.Set("speaker", strconv.Itoa(styleID))
	payload, err := json.Marshal(query)
	if err != nil {
		return nil, toolerr.Newf(toolerr.CodeEngineRequest, "marshal audio query: %v", err)
	}
	return c.do(ctx, http.MethodPost, "/synthesis", q, payload)
}

// AddUserDictWord registers one dictionary word and returns its engine-side
// UUID (POST /user_dict_word — parameters go in the query string, not the
// body, per the VOICEVOX API).
func (c *Client) AddUserDictWord(ctx context.Context, w UserDictWord) (string, error) {
	q := url.Values{}
	q.Set("surface", w.Surface)
	q.Set("pronunciation", w.Pronunciation)
	q.Set("accent_type", strconv.Itoa(w.AccentType))
	if w.WordType != "" {
		q.Set("word_type", w.WordType)
	}
	if w.Priority != nil {
		q.Set("priority", strconv.Itoa(*w.Priority))
	}
	body, err := c.do(ctx, http.MethodPost, "/user_dict_word", q, nil)
	if err != nil {
		return "", err
	}
	var uuid string
	if err := json.Unmarshal(body, &uuid); err != nil {
		return "", toolerr.Newf(toolerr.CodeEngineRequest, "parse /user_dict_word response: %v", err)
	}
	return uuid, nil
}

// do performs one request and maps failures to structured errors:
// transport-level failures → engine_unavailable, non-2xx → engine_request_failed.
func (c *Client) do(ctx context.Context, method, path string, query url.Values, payload []byte) ([]byte, error) {
	u := c.baseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	var rd io.Reader
	if payload != nil {
		rd = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rd)
	if err != nil {
		return nil, toolerr.Newf(toolerr.CodeEngineRequest, "build request: %v", err)
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, toolerr.Newf(toolerr.CodeEngineUnavailable,
			"engine not reachable at %s: %v", c.baseURL, err).WithDetails(map[string]any{
			"url": u,
		})
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, toolerr.Newf(toolerr.CodeEngineRequest, "read response: %v", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		head := body
		if len(head) > errBodyHead {
			head = head[:errBodyHead]
		}
		return nil, toolerr.Newf(toolerr.CodeEngineRequest,
			"engine returned HTTP %d for %s %s", resp.StatusCode, method, path).WithDetails(map[string]any{
			"status":    resp.StatusCode,
			"path":      path,
			"body_head": string(head),
		})
	}
	return body, nil
}
