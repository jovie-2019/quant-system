package contracts

// Kline represents an OHLCV candlestick.
type Kline struct {
	Venue     Venue   `json:"venue"`
	Symbol    string  `json:"symbol"`
	Interval  string  `json:"interval"` // "1m","5m","15m","1h","4h","1d"
	OpenTime  int64   `json:"open_time"`
	Open      float64 `json:"open"`
	High      float64 `json:"high"`
	Low       float64 `json:"low"`
	Close     float64 `json:"close"`
	Volume    float64 `json:"volume"`
	CloseTime int64   `json:"close_time"`
	Closed    bool    `json:"closed"`
}

// DepthLevel is a single price level in the order book.
type DepthLevel struct {
	Price    float64 `json:"price"`
	Quantity float64 `json:"quantity"`
}

// DepthSnapshot is a 20-level order book snapshot.
type DepthSnapshot struct {
	Venue  Venue        `json:"venue"`
	Symbol string       `json:"symbol"`
	Bids   []DepthLevel `json:"bids"` // price descending
	Asks   []DepthLevel `json:"asks"` // price ascending
	TSms   int64        `json:"ts_ms"`
}
