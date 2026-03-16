package rpc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"dustcleaner/config"
	"dustcleaner/utxo"
)

// BitcoinRPC is a JSON-RPC client for Bitcoin Core.
type BitcoinRPC struct {
	baseURL  string // Base URL without wallet path
	wallet   string // Wallet name (empty for node-level calls)
	username string
	password string
	client   *http.Client
}

// NewFromConfig constructs a BitcoinRPC from a Config object.
func NewFromConfig(cfg *config.Config) *BitcoinRPC {
	return &BitcoinRPC{
		baseURL:  baseURL,
		wallet:   cfg.Wallet,
		username: cfg.RPCUser,
		password: cfg.RPCPass,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// getURL returns the full RPC URL for the given method.
func (c *BitcoinRPC) getURL(method string) (string, error) {
	walletMethods := map[string]bool{
		"listunspent":        true,
		"getnewaddress":      true,
		"sendtoaddress":      true,
		"getbalance":         true,
		"listtransactions":   true,
		"listaddresses":     true,
		"getaddressinfo":    true,
		"walletprocesspsbt":  true,
		"finalizepsbt":       true,
		"walletcreatefundedpsbt": true,
	}
	
	baseURL, err := url.Parse(c.baseURL)
	if err != nil {
		return "", fmt.Errorf("parse base URL: %w", err)
	}
	
	if walletMethods[method] && c.wallet != "" {
		path := strings.TrimSuffix(baseURL.Path, "/")
		baseURL.Path = path + "/wallet/" + c.wallet
	}
	
	return baseURL.String(), nil
}

// rpcRequest represents a JSON-RPC 1.0 request.
type rpcRequest struct {
	JSONRPC string        `json:"jsonrpc"`
	ID      string        `json:"id"`
	Method  string        `json:"method"`
	Params  []interface{} `json:"params"`
}

// rpcResponse is a generic JSON-RPC 1.0 response wrapper.
type rpcResponse[T any] struct {
	Result T           `json:"result"`
	Error  *rpcError   `json:"error"`
	ID     interface{} `json:"id"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// do sends a JSON-RPC request and decodes the typed result into out.
func (c *BitcoinRPC) do(ctx context.Context, method string, params []interface{}, out interface{}) error {
	reqBody, err := json.Marshal(rpcRequest{
		JSONRPC: "1.0",
		ID:      "dustcleaner",
		Method:  method,
		Params:  params,
	})
	if err != nil {
		return fmt.Errorf("marshal rpc request: %w", err)
	}

	rpcURL, err := c.getURL(method)
	if err != nil {
		return fmt.Errorf("get RPC URL: %w", err)
	}
	
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, rpcURL, bytes.NewReader(reqBody))
	if err != nil {
		return fmt.Errorf("create http request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.username != "" && c.password != "" {
		httpReq.SetBasicAuth(c.username, c.password)
	}

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("perform http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected http status: %s", resp.Status)
	}

	dec := json.NewDecoder(resp.Body)
	if err := dec.Decode(out); err != nil {
		return fmt.Errorf("decode rpc response: %w", err)
	}

	return nil
}

// ListUnspent returns all wallet UTXOs as internal utxo.UTXO values.
func (c *BitcoinRPC) ListUnspent() ([]utxo.UTXO, error) {
	ctx := context.Background()

	var resp rpcResponse[[]utxo.UTXO]
	if err := c.do(ctx, "listunspent", nil, &resp); err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("rpc error %d: %s", resp.Error.Code, resp.Error.Message)
	}

	return resp.Result, nil
}

// GetRawTransaction returns the raw transaction hex string for the given txid.
func (c *BitcoinRPC) GetRawTransaction(txid string) (string, error) {
	ctx := context.Background()

	var resp rpcResponse[string]
	if err := c.do(ctx, "getrawtransaction", []interface{}{txid}, &resp); err != nil {
		return "", err
	}
	if resp.Error != nil {
		return "", fmt.Errorf("rpc error %d: %s", resp.Error.Code, resp.Error.Message)
	}

	return resp.Result, nil
}

// GetMempoolInfo queries getmempoolinfo and returns the raw JSON response as a map.
func (c *BitcoinRPC) GetMempoolInfo() (map[string]any, error) {
	ctx := context.Background()

	var resp rpcResponse[map[string]any]
	if err := c.do(ctx, "getmempoolinfo", nil, &resp); err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("rpc error %d: %s", resp.Error.Code, resp.Error.Message)
	}

	return resp.Result, nil
}

// GetRawMempool returns a list of transaction IDs in the mempool.
func (c *BitcoinRPC) GetRawMempool() ([]string, error) {
	ctx := context.Background()

	var resp rpcResponse[[]string]
	if err := c.do(ctx, "getrawmempool", []interface{}{false}, &resp); err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("rpc error %d: %s", resp.Error.Code, resp.Error.Message)
	}

	return resp.Result, nil
}

// GetMempoolEntry returns mempool data for a specific transaction.
func (c *BitcoinRPC) GetMempoolEntry(txid string) (map[string]any, error) {
	ctx := context.Background()

	var resp rpcResponse[map[string]any]
	if err := c.do(ctx, "getmempoolentry", []interface{}{txid}, &resp); err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("rpc error %d: %s", resp.Error.Code, resp.Error.Message)
	}

	return resp.Result, nil
}

// GetNewAddress returns a new address from the wallet.
func (c *BitcoinRPC) GetNewAddress(label, addrType string) (string, error) {
	ctx := context.Background()

	params := []interface{}{label}
	if addrType != "" {
		params = append(params, addrType)
	}

	var resp rpcResponse[string]
	if err := c.do(ctx, "getnewaddress", params, &resp); err != nil {
		return "", err
	}
	if resp.Error != nil {
		return "", fmt.Errorf("rpc error %d: %s", resp.Error.Code, resp.Error.Message)
	}

	return resp.Result, nil
}

// GenerateToAddress mines blocks to the given address (regtest/signet only).
func (c *BitcoinRPC) GenerateToAddress(blocks int, address string) ([]string, error) {
	ctx := context.Background()

	var resp rpcResponse[[]string]
	if err := c.do(ctx, "generatetoaddress", []interface{}{blocks, address}, &resp); err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("rpc error %d: %s", resp.Error.Code, resp.Error.Message)
	}

	return resp.Result, nil
}

// SendToAddress sends amountSats (in satoshis) to the given address.
func (c *BitcoinRPC) SendToAddress(address string, amountSats int64, feeRateSatPerVByte float64) (string, error) {
	ctx := context.Background()

	amountBTC := float64(amountSats) / 1e8
	params := []interface{}{address, amountBTC}
	if feeRateSatPerVByte > 0 {
		params = append(params, "", "", false, false, nil, "unset", false, feeRateSatPerVByte)
	}

	var resp rpcResponse[string]
	if err := c.do(ctx, "sendtoaddress", params, &resp); err != nil {
		return "", err
	}
	if resp.Error != nil {
		return "", fmt.Errorf("rpc error %d: %s", resp.Error.Code, resp.Error.Message)
	}

	return resp.Result, nil
}

// ConvertToPSBT converts a transaction hex to PSBT format.
func (c *BitcoinRPC) ConvertToPSBT(rawTxHex string) (string, error) {
	ctx := context.Background()

	var resp rpcResponse[string]
	if err := c.do(ctx, "converttopsbt", []interface{}{rawTxHex}, &resp); err != nil {
		return "", err
	}
	if resp.Error != nil {
		return "", fmt.Errorf("rpc error %d: %s", resp.Error.Code, resp.Error.Message)
	}

	return resp.Result, nil
}
