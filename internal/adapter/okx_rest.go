package adapter

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var (
	ErrOKXConfigInvalid = errors.New("adapter/okx: invalid config")
	ErrOKXResponse      = errors.New("adapter/okx: unexpected response")
)

type OKXSpotRESTConfig struct {
	BaseURL          string
	APIKey           string
	APISecret        string
	Passphrase       string
	MinInterval      time.Duration
	SimulatedTrading bool
}

type OKXSpotTradeGateway struct {
	baseURL          string
	apiKey           string
	apiSecret        string
	passphrase       string
	client           *http.Client
	now              func() time.Time
	pacer            *requestPacer
	simulatedTrading bool
}

func NewOKXSpotTradeGateway(cfg OKXSpotRESTConfig, client *http.Client) (*OKXSpotTradeGateway, error) {
	if strings.TrimSpace(cfg.BaseURL) == "" ||
		strings.TrimSpace(cfg.APIKey) == "" ||
		strings.TrimSpace(cfg.APISecret) == "" ||
		strings.TrimSpace(cfg.Passphrase) == "" {
		return nil, ErrOKXConfigInvalid
	}
	if client == nil {
		client = newHTTPClient(8 * time.Second)
	}
	return &OKXSpotTradeGateway{
		baseURL:          strings.TrimRight(cfg.BaseURL, "/"),
		apiKey:           strings.TrimSpace(cfg.APIKey),
		apiSecret:        strings.TrimSpace(cfg.APISecret),
		passphrase:       strings.TrimSpace(cfg.Passphrase),
		client:           client,
		now:              time.Now,
		pacer:            newRequestPacer(cfg.MinInterval),
		simulatedTrading: cfg.SimulatedTrading,
	}, nil
}

func (g *OKXSpotTradeGateway) PlaceOrder(ctx context.Context, req VenueOrderRequest) (VenueOrderAck, error) {
	if err := validateOrderRequest(req); err != nil {
		return VenueOrderAck{}, err
	}

	if err := g.pacer.wait(ctx, g.now); err != nil {
		return VenueOrderAck{}, err
	}

	path := "/api/v5/trade/order"
	payload := map[string]string{
		"instId":  canonicalToOKXInstID(req.Symbol),
		"tdMode":  "cash",
		"side":    strings.ToLower(strings.TrimSpace(req.Side)),
		"ordType": "limit",
		"px":      formatDecimal(req.Price),
		"sz":      formatDecimal(req.Quantity),
		"clOrdId": strings.TrimSpace(req.ClientOrderID),
	}

	body, err := g.signedJSONRequest(ctx, http.MethodPost, path, payload)
	if err != nil {
		return VenueOrderAck{}, err
	}

	var resp struct {
		Code string `json:"code"`
		Msg  string `json:"msg"`
		Data []struct {
			OrdID   string `json:"ordId"`
			ClOrdID string `json:"clOrdId"`
			SCode   string `json:"sCode"`
			SMsg    string `json:"sMsg"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return VenueOrderAck{}, asNonRetryable(fmt.Errorf("%w: decode place order: %v", ErrOKXResponse, err))
	}
	if resp.Code != "0" || len(resp.Data) == 0 {
		return VenueOrderAck{}, asNonRetryable(fmt.Errorf("%w: code=%s msg=%s", ErrOKXResponse, resp.Code, resp.Msg))
	}
	if resp.Data[0].SCode != "" && resp.Data[0].SCode != "0" {
		return VenueOrderAck{}, asNonRetryable(fmt.Errorf("%w: scode=%s smsg=%s", ErrOKXResponse, resp.Data[0].SCode, resp.Data[0].SMsg))
	}

	clientOrderID := resp.Data[0].ClOrdID
	if strings.TrimSpace(clientOrderID) == "" {
		clientOrderID = req.ClientOrderID
	}

	return VenueOrderAck{
		ClientOrderID: clientOrderID,
		VenueOrderID:  resp.Data[0].OrdID,
		Status:        "live",
	}, nil
}

func (g *OKXSpotTradeGateway) CancelOrder(ctx context.Context, req VenueCancelRequest) (VenueCancelAck, error) {
	if err := validateCancelRequest(req); err != nil {
		return VenueCancelAck{}, err
	}

	if err := g.pacer.wait(ctx, g.now); err != nil {
		return VenueCancelAck{}, err
	}

	path := "/api/v5/trade/cancel-order"
	payload := map[string]string{
		"instId": canonicalToOKXInstID(req.Symbol),
	}
	if strings.TrimSpace(req.ClientOrderID) != "" {
		payload["clOrdId"] = strings.TrimSpace(req.ClientOrderID)
	}
	if strings.TrimSpace(req.VenueOrderID) != "" {
		payload["ordId"] = strings.TrimSpace(req.VenueOrderID)
	}

	body, err := g.signedJSONRequest(ctx, http.MethodPost, path, payload)
	if err != nil {
		return VenueCancelAck{}, err
	}

	var resp struct {
		Code string `json:"code"`
		Msg  string `json:"msg"`
		Data []struct {
			OrdID   string `json:"ordId"`
			ClOrdID string `json:"clOrdId"`
			SCode   string `json:"sCode"`
			SMsg    string `json:"sMsg"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return VenueCancelAck{}, asNonRetryable(fmt.Errorf("%w: decode cancel order: %v", ErrOKXResponse, err))
	}
	if resp.Code != "0" || len(resp.Data) == 0 {
		return VenueCancelAck{}, asNonRetryable(fmt.Errorf("%w: code=%s msg=%s", ErrOKXResponse, resp.Code, resp.Msg))
	}
	if resp.Data[0].SCode != "" && resp.Data[0].SCode != "0" {
		return VenueCancelAck{}, asNonRetryable(fmt.Errorf("%w: scode=%s smsg=%s", ErrOKXResponse, resp.Data[0].SCode, resp.Data[0].SMsg))
	}

	return VenueCancelAck{
		ClientOrderID: firstNonEmptyString(resp.Data[0].ClOrdID, req.ClientOrderID),
		VenueOrderID:  firstNonEmptyString(resp.Data[0].OrdID, req.VenueOrderID),
		Status:        "canceled",
	}, nil
}

func (g *OKXSpotTradeGateway) QueryOrder(ctx context.Context, req VenueOrderQueryRequest) (VenueOrderStatus, error) {
	if err := validateQueryRequest(req); err != nil {
		return VenueOrderStatus{}, err
	}
	if err := g.pacer.wait(ctx, g.now); err != nil {
		return VenueOrderStatus{}, err
	}

	path := "/api/v5/trade/order"
	query := url.Values{}
	query.Set("instId", canonicalToOKXInstID(req.Symbol))
	if clientOrderID := strings.TrimSpace(req.ClientOrderID); clientOrderID != "" {
		query.Set("clOrdId", clientOrderID)
	} else if venueOrderID := strings.TrimSpace(req.VenueOrderID); venueOrderID != "" {
		query.Set("ordId", venueOrderID)
	}

	body, err := g.signedGETRequest(ctx, path, query)
	if err != nil {
		return VenueOrderStatus{}, err
	}

	var resp struct {
		Code string `json:"code"`
		Msg  string `json:"msg"`
		Data []struct {
			OrdID     string `json:"ordId"`
			ClOrdID   string `json:"clOrdId"`
			InstID    string `json:"instId"`
			State     string `json:"state"`
			AccFillSz string `json:"accFillSz"`
			FillSz    string `json:"fillSz"`
			AvgPx     string `json:"avgPx"`
			FillPx    string `json:"fillPx"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return VenueOrderStatus{}, asNonRetryable(fmt.Errorf("%w: decode query order: %v", ErrOKXResponse, err))
	}
	if resp.Code != "0" || len(resp.Data) == 0 {
		return VenueOrderStatus{}, asNonRetryable(fmt.Errorf("%w: code=%s msg=%s", ErrOKXResponse, resp.Code, resp.Msg))
	}

	filledQty := parseDecimal(resp.Data[0].AccFillSz)
	if filledQty <= 0 {
		filledQty = parseDecimal(resp.Data[0].FillSz)
	}
	avgPrice := parseDecimal(resp.Data[0].AvgPx)
	if avgPrice <= 0 {
		avgPrice = parseDecimal(resp.Data[0].FillPx)
	}

	return VenueOrderStatus{
		ClientOrderID: firstNonEmptyString(resp.Data[0].ClOrdID, req.ClientOrderID),
		VenueOrderID:  firstNonEmptyString(resp.Data[0].OrdID, req.VenueOrderID),
		Symbol:        firstNonEmptyString(resp.Data[0].InstID, normalizeCanonicalSymbol(req.Symbol)),
		Status:        strings.ToLower(strings.TrimSpace(resp.Data[0].State)),
		FilledQty:     filledQty,
		AvgPrice:      avgPrice,
	}, nil
}

func (g *OKXSpotTradeGateway) signedJSONRequest(ctx context.Context, method string, path string, payload any) ([]byte, error) {
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	timestamp := g.now().UTC().Format("2006-01-02T15:04:05.000Z")
	signature := g.sign(timestamp + method + path + string(bodyBytes))

	req, err := http.NewRequestWithContext(ctx, method, g.baseURL+path, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("OK-ACCESS-KEY", g.apiKey)
	req.Header.Set("OK-ACCESS-SIGN", signature)
	req.Header.Set("OK-ACCESS-TIMESTAMP", timestamp)
	req.Header.Set("OK-ACCESS-PASSPHRASE", g.passphrase)
	if g.simulatedTrading {
		req.Header.Set("x-simulated-trading", "1")
	}

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
		return nil, classifyHTTPError(ErrOKXResponse, resp.StatusCode, string(body))
	}
	return body, nil
}

func (g *OKXSpotTradeGateway) signedGETRequest(ctx context.Context, path string, query url.Values) ([]byte, error) {
	requestPath := path
	if encoded := query.Encode(); encoded != "" {
		requestPath = path + "?" + encoded
	}
	timestamp := g.now().UTC().Format("2006-01-02T15:04:05.000Z")
	signature := g.sign(timestamp + http.MethodGet + requestPath)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, g.baseURL+requestPath, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("OK-ACCESS-KEY", g.apiKey)
	req.Header.Set("OK-ACCESS-SIGN", signature)
	req.Header.Set("OK-ACCESS-TIMESTAMP", timestamp)
	req.Header.Set("OK-ACCESS-PASSPHRASE", g.passphrase)
	if g.simulatedTrading {
		req.Header.Set("x-simulated-trading", "1")
	}

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
		return nil, classifyHTTPError(ErrOKXResponse, resp.StatusCode, string(body))
	}
	return body, nil
}

func (g *OKXSpotTradeGateway) sign(payload string) string {
	mac := hmac.New(sha256.New, []byte(g.apiSecret))
	_, _ = mac.Write([]byte(payload))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
