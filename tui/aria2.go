package tui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type aria2Client struct {
	url    string
	secret string
}

type rpcRequest struct {
	JSONRPC string        `json:"jsonrpc"`
	Method  string        `json:"method"`
	Params  []interface{} `json:"params,omitempty"`
	ID      int           `json:"id"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	Result  json.RawMessage `json:"result"`
	Error   *rpcError       `json:"error"`
	ID      int             `json:"id"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type tellStatusResult struct {
	GID             string        `json:"gid"`
	Status          string        `json:"status"`
	TotalLength     string        `json:"totalLength"`
	CompletedLength string        `json:"completedLength"`
	DownloadSpeed   string        `json:"downloadSpeed"`
	UploadSpeed     string        `json:"uploadSpeed"`
	Files           []aria2File   `json:"files"`
	Bittorrent      *btInfo       `json:"bittorrent"`
	ErrorMessage    string        `json:"errorMessage"`
	ErrorCode       string        `json:"errorCode"`
}

type aria2File struct {
	Path   string `json:"path"`
	Length string `json:"length"`
}

type btInfo struct {
	Info struct {
		Name string `json:"name"`
	} `json:"info"`
}

func newAria2Client(port int, secret string) *aria2Client {
	return &aria2Client{
		url:    fmt.Sprintf("http://127.0.0.1:%d/jsonrpc", port),
		secret: secret,
	}
}

func (c *aria2Client) call(method string, params ...interface{}) (json.RawMessage, error) {
	req := rpcRequest{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
		ID:      1,
	}
	body, _ := json.Marshal(req)
	httpReq, _ := http.NewRequest("POST", c.url, bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	if c.secret != "" {
		httpReq.Header.Set("X-Aria2-Token", c.secret)
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("aria2 rpc: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1*1024*1024))

	var rpcResp rpcResponse
	if err := json.Unmarshal(respBody, &rpcResp); err != nil {
		return nil, fmt.Errorf("aria2 parse: %w", err)
	}
	if rpcResp.Error != nil {
		return nil, fmt.Errorf("aria2 error %d: %s", rpcResp.Error.Code, rpcResp.Error.Message)
	}
	return rpcResp.Result, nil
}

func (c *aria2Client) AddURI(magnet, dir string) (string, error) {
	opts := map[string]interface{}{}
	if dir != "" {
		opts["dir"] = dir
	}
	params := []interface{}{[]string{magnet}, opts}
	res, err := c.call("aria2.addUri", params...)
	if err != nil {
		return "", err
	}
	var gid string
	json.Unmarshal(res, &gid)
	return gid, nil
}

func (c *aria2Client) Pause(gid string) error {
	_, err := c.call("aria2.pause", gid)
	return err
}

func (c *aria2Client) Unpause(gid string) error {
	_, err := c.call("aria2.unpause", gid)
	return err
}

func (c *aria2Client) Remove(gid string) error {
	_, err := c.call("aria2.remove", gid)
	return err
}

func (c *aria2Client) ForceRemove(gid string) error {
	_, err := c.call("aria2.forceRemove", gid)
	return err
}

func (c *aria2Client) RemoveResult(gid string) error {
	_, err := c.call("aria2.removeDownloadResult", gid)
	return err
}

func (c *aria2Client) TellStatus(gid string) (*tellStatusResult, error) {
	res, err := c.call("aria2.tellStatus", gid)
	if err != nil {
		return nil, err
	}
	var ts tellStatusResult
	if err := json.Unmarshal(res, &ts); err != nil {
		return nil, err
	}
	return &ts, nil
}

func (c *aria2Client) TellActive() ([]tellStatusResult, error) {
	res, err := c.call("aria2.tellActive")
	if err != nil {
		return nil, err
	}
	var results []tellStatusResult
	if err := json.Unmarshal(res, &results); err != nil {
		return nil, err
	}
	return results, nil
}

func (c *aria2Client) TellWaiting(offset, num int) ([]tellStatusResult, error) {
	res, err := c.call("aria2.tellWaiting", offset, num)
	if err != nil {
		return nil, err
	}
	var results []tellStatusResult
	if err := json.Unmarshal(res, &results); err != nil {
		return nil, err
	}
	return results, nil
}

func (c *aria2Client) TellStopped(offset, num int) ([]tellStatusResult, error) {
	res, err := c.call("aria2.tellStopped", offset, num)
	if err != nil {
		return nil, err
	}
	var results []tellStatusResult
	if err := json.Unmarshal(res, &results); err != nil {
		return nil, err
	}
	return results, nil
}

func parseLength(s string) int64 {
	n, _ := strconv.ParseInt(s, 10, 64)
	return n
}

func progressPct(completed, total int64) float64 {
	if total == 0 {
		return 0
	}
	return float64(completed) / float64(total) * 100
}

func progressBar(pct float64, width int) string {
	filled := int(pct / 100 * float64(width))
	if filled < 0 {
		filled = 0
	}
	if filled > width {
		filled = width
	}
	return strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
}
