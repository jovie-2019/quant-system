package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

// AssetBalance represents the balance of a single asset on an exchange.
type AssetBalance struct {
	Asset  string  `json:"asset"`
	Free   float64 `json:"free"`
	Locked float64 `json:"locked"`
	Total  float64 `json:"total"`
}

// QueryBalance queries all non-zero asset balances from Binance spot account.
func (g *BinanceSpotTradeGateway) QueryBalance(ctx context.Context) ([]AssetBalance, error) {
	if err := g.pacer.wait(ctx, g.now); err != nil {
		return nil, err
	}

	params := url.Values{}
	params.Set("recvWindow", strconv.FormatInt(g.recvWindowMS, 10))
	params.Set("timestamp", strconv.FormatInt(g.now().UnixMilli(), 10))

	endpoint := g.baseURL + "/api/v3/account"
	body, err := g.signedRequest(ctx, http.MethodGet, endpoint, params)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Balances []struct {
			Asset  string `json:"asset"`
			Free   string `json:"free"`
			Locked string `json:"locked"`
		} `json:"balances"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("%w: decode account balance: %v", ErrBinanceResponse, err)
	}

	out := make([]AssetBalance, 0, len(resp.Balances))
	for _, b := range resp.Balances {
		free := parseDecimal(b.Free)
		locked := parseDecimal(b.Locked)
		if free == 0 && locked == 0 {
			continue
		}
		out = append(out, AssetBalance{
			Asset:  b.Asset,
			Free:   free,
			Locked: locked,
			Total:  free + locked,
		})
	}
	return out, nil
}

// QueryBalance queries all non-zero asset balances from OKX account.
func (g *OKXSpotTradeGateway) QueryBalance(ctx context.Context) ([]AssetBalance, error) {
	if err := g.pacer.wait(ctx, g.now); err != nil {
		return nil, err
	}

	path := "/api/v5/account/balance"
	body, err := g.signedGETRequest(ctx, path, url.Values{})
	if err != nil {
		return nil, err
	}

	var resp struct {
		Code string `json:"code"`
		Msg  string `json:"msg"`
		Data []struct {
			Details []struct {
				Ccy       string `json:"ccy"`
				AvailBal  string `json:"availBal"`
				FrozenBal string `json:"frozenBal"`
			} `json:"details"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("%w: decode account balance: %v", ErrOKXResponse, err)
	}
	if resp.Code != "0" {
		return nil, fmt.Errorf("%w: code=%s msg=%s", ErrOKXResponse, resp.Code, resp.Msg)
	}
	if len(resp.Data) == 0 {
		return nil, nil
	}

	details := resp.Data[0].Details
	out := make([]AssetBalance, 0, len(details))
	for _, d := range details {
		free := parseDecimal(d.AvailBal)
		locked := parseDecimal(d.FrozenBal)
		if free == 0 && locked == 0 {
			continue
		}
		out = append(out, AssetBalance{
			Asset:  d.Ccy,
			Free:   free,
			Locked: locked,
			Total:  free + locked,
		})
	}
	return out, nil
}
