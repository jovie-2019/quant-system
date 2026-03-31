package adapter

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var (
	ErrBinanceConfigInvalid = errors.New("adapter/binance: invalid config")
	ErrBinanceResponse      = errors.New("adapter/binance: unexpected response")
)

type BinanceSpotRESTConfig struct {
	BaseURL      string
	APIKey       string
	APISecret    string
	RecvWindowMS int64
	MinInterval  time.Duration
}

type BinanceSpotTradeGateway struct {
	baseURL      string
	apiKey       string
	apiSecret    string
	recvWindowMS int64
	client       *http.Client
	now          func() time.Time
	pacer        *requestPacer
}

func NewBinanceSpotTradeGateway(cfg BinanceSpotRESTConfig, client *http.Client) (*BinanceSpotTradeGateway, error) {
	if strings.TrimSpace(cfg.BaseURL) == "" || strings.TrimSpace(cfg.APIKey) == "" || strings.TrimSpace(cfg.APISecret) == "" {
		return nil, ErrBinanceConfigInvalid
	}
	if cfg.RecvWindowMS <= 0 {
		cfg.RecvWindowMS = 5000
	}
	if client == nil {
		client = &http.Client{Timeout: 8 * time.Second}
	}

	return &BinanceSpotTradeGateway{
		baseURL:      strings.TrimRight(cfg.BaseURL, "/"),
		apiKey:       strings.TrimSpace(cfg.APIKey),
		apiSecret:    strings.TrimSpace(cfg.APISecret),
		recvWindowMS: cfg.RecvWindowMS,
		client:       client,
		now:          time.Now,
		pacer:        newRequestPacer(cfg.MinInterval),
	}, nil
}

func (g *BinanceSpotTradeGateway) PlaceOrder(ctx context.Context, req VenueOrderRequest) (VenueOrderAck, error) {
	if err := validateOrderRequest(req); err != nil {
		return VenueOrderAck{}, err
	}

	if err := g.pacer.wait(ctx, g.now); err != nil {
		return VenueOrderAck{}, err
	}

	params := url.Values{}
	params.Set("symbol", canonicalToBinanceSymbol(req.Symbol))
	params.Set("side", strings.ToUpper(strings.TrimSpace(req.Side)))
	params.Set("type", "LIMIT")
	params.Set("timeInForce", "GTC")
	params.Set("quantity", formatDecimal(req.Quantity))
	params.Set("price", formatDecimal(req.Price))
	params.Set("newClientOrderId", strings.TrimSpace(req.ClientOrderID))
	params.Set("recvWindow", strconv.FormatInt(g.recvWindowMS, 10))
	params.Set("timestamp", strconv.FormatInt(g.now().UnixMilli(), 10))

	endpoint := g.baseURL + "/api/v3/order"
	body, err := g.signedRequest(ctx, http.MethodPost, endpoint, params)
	if err != nil {
		return VenueOrderAck{}, err
	}

	var resp struct {
		Symbol        string `json:"symbol"`
		OrderID       int64  `json:"orderId"`
		ClientOrderID string `json:"clientOrderId"`
		Status        string `json:"status"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return VenueOrderAck{}, fmt.Errorf("%w: decode place order: %v", ErrBinanceResponse, err)
	}

	clientOrderID := strings.TrimSpace(resp.ClientOrderID)
	if clientOrderID == "" {
		clientOrderID = req.ClientOrderID
	}

	return VenueOrderAck{
		ClientOrderID: clientOrderID,
		VenueOrderID:  strconv.FormatInt(resp.OrderID, 10),
		Status:        strings.ToLower(strings.TrimSpace(resp.Status)),
	}, nil
}

func (g *BinanceSpotTradeGateway) CancelOrder(ctx context.Context, req VenueCancelRequest) (VenueCancelAck, error) {
	if err := validateCancelRequest(req); err != nil {
		return VenueCancelAck{}, err
	}

	if err := g.pacer.wait(ctx, g.now); err != nil {
		return VenueCancelAck{}, err
	}

	params := url.Values{}
	params.Set("symbol", canonicalToBinanceSymbol(req.Symbol))
	if clientOrderID := strings.TrimSpace(req.ClientOrderID); clientOrderID != "" {
		params.Set("origClientOrderId", clientOrderID)
	} else if venueOrderID := strings.TrimSpace(req.VenueOrderID); venueOrderID != "" {
		params.Set("orderId", venueOrderID)
	}
	params.Set("recvWindow", strconv.FormatInt(g.recvWindowMS, 10))
	params.Set("timestamp", strconv.FormatInt(g.now().UnixMilli(), 10))

	endpoint := g.baseURL + "/api/v3/order"
	body, err := g.signedRequest(ctx, http.MethodDelete, endpoint, params)
	if err != nil {
		return VenueCancelAck{}, err
	}

	var resp struct {
		Symbol        string `json:"symbol"`
		OrderID       int64  `json:"orderId"`
		ClientOrderID string `json:"clientOrderId"`
		Status        string `json:"status"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return VenueCancelAck{}, fmt.Errorf("%w: decode cancel order: %v", ErrBinanceResponse, err)
	}

	status := strings.ToLower(strings.TrimSpace(resp.Status))
	if status == "" {
		status = "canceled"
	}

	clientOrderID := strings.TrimSpace(resp.ClientOrderID)
	if clientOrderID == "" {
		clientOrderID = req.ClientOrderID
	}

	venueOrderID := strings.TrimSpace(req.VenueOrderID)
	if venueOrderID == "" && resp.OrderID > 0 {
		venueOrderID = strconv.FormatInt(resp.OrderID, 10)
	}

	return VenueCancelAck{
		ClientOrderID: clientOrderID,
		VenueOrderID:  venueOrderID,
		Status:        status,
	}, nil
}

func (g *BinanceSpotTradeGateway) QueryOrder(ctx context.Context, req VenueOrderQueryRequest) (VenueOrderStatus, error) {
	if err := validateQueryRequest(req); err != nil {
		return VenueOrderStatus{}, err
	}
	if err := g.pacer.wait(ctx, g.now); err != nil {
		return VenueOrderStatus{}, err
	}

	params := url.Values{}
	params.Set("symbol", canonicalToBinanceSymbol(req.Symbol))
	if clientOrderID := strings.TrimSpace(req.ClientOrderID); clientOrderID != "" {
		params.Set("origClientOrderId", clientOrderID)
	} else if venueOrderID := strings.TrimSpace(req.VenueOrderID); venueOrderID != "" {
		params.Set("orderId", venueOrderID)
	}
	params.Set("recvWindow", strconv.FormatInt(g.recvWindowMS, 10))
	params.Set("timestamp", strconv.FormatInt(g.now().UnixMilli(), 10))

	endpoint := g.baseURL + "/api/v3/order"
	body, err := g.signedRequest(ctx, http.MethodGet, endpoint, params)
	if err != nil {
		return VenueOrderStatus{}, err
	}

	var resp struct {
		Symbol             string `json:"symbol"`
		OrderID            int64  `json:"orderId"`
		ClientOrderID      string `json:"clientOrderId"`
		Status             string `json:"status"`
		ExecutedQty        string `json:"executedQty"`
		CumulativeQuoteQty string `json:"cummulativeQuoteQty"`
		OrigQty            string `json:"origQty"`
		Price              string `json:"price"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return VenueOrderStatus{}, fmt.Errorf("%w: decode query order: %v", ErrBinanceResponse, err)
	}

	filledQty := parseDecimal(resp.ExecutedQty)
	avgPrice := 0.0
	if filledQty > 0 {
		quoteQty := parseDecimal(resp.CumulativeQuoteQty)
		if quoteQty > 0 {
			avgPrice = quoteQty / filledQty
		}
	}
	if avgPrice <= 0 {
		avgPrice = parseDecimal(resp.Price)
	}

	clientOrderID := strings.TrimSpace(resp.ClientOrderID)
	if clientOrderID == "" {
		clientOrderID = strings.TrimSpace(req.ClientOrderID)
	}
	venueOrderID := strings.TrimSpace(req.VenueOrderID)
	if venueOrderID == "" && resp.OrderID > 0 {
		venueOrderID = strconv.FormatInt(resp.OrderID, 10)
	}

	return VenueOrderStatus{
		ClientOrderID: clientOrderID,
		VenueOrderID:  venueOrderID,
		Symbol:        normalizeCanonicalSymbol(resp.Symbol),
		Status:        strings.ToLower(strings.TrimSpace(resp.Status)),
		FilledQty:     filledQty,
		AvgPrice:      avgPrice,
	}, nil
}

func (g *BinanceSpotTradeGateway) signedRequest(ctx context.Context, method string, endpoint string, params url.Values) ([]byte, error) {
	query := params.Encode()
	signature := g.sign(query)

	reqURL := endpoint + "?" + query + "&signature=" + signature
	req, err := http.NewRequestWithContext(ctx, method, reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-MBX-APIKEY", g.apiKey)

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, classifyTransportError(err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, classifyHTTPError(ErrBinanceResponse, resp.StatusCode, string(body))
	}
	return body, nil
}

func (g *BinanceSpotTradeGateway) sign(payload string) string {
	mac := hmac.New(sha256.New, []byte(g.apiSecret))
	_, _ = mac.Write([]byte(payload))
	return hex.EncodeToString(mac.Sum(nil))
}
