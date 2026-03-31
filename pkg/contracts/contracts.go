package contracts

type Venue string

const (
	VenueBinance Venue = "binance"
	VenueOKX     Venue = "okx"
)

type RawMarketEvent struct {
	Venue      Venue
	Symbol     string
	EventType  string
	Payload    []byte
	SourceTSMS int64
	Sequence   int64
}

type RawExecEvent struct {
	Venue      Venue
	Symbol     string
	Payload    []byte
	SourceTSMS int64
}

type VenueOrderRequest struct {
	ClientOrderID string
	Symbol        string
	Side          string
	Price         float64
	Quantity      float64
}

type VenueOrderAck struct {
	ClientOrderID string
	VenueOrderID  string
	Status        string
}

type VenueCancelRequest struct {
	ClientOrderID string
	VenueOrderID  string
	Symbol        string
}

type VenueCancelAck struct {
	ClientOrderID string
	VenueOrderID  string
	Status        string
}

type VenueOrderQueryRequest struct {
	ClientOrderID string
	VenueOrderID  string
	Symbol        string
}

type VenueOrderStatus struct {
	ClientOrderID string
	VenueOrderID  string
	Symbol        string
	Status        string
	FilledQty     float64
	AvgPrice      float64
}

type MarketNormalizedEvent struct {
	Venue      Venue
	Symbol     string
	Sequence   int64
	BidPX      float64
	BidSZ      float64
	AskPX      float64
	AskSZ      float64
	LastPX     float64
	SourceTSMS int64
	IngestTSMS int64
	EmitTSMS   int64
}

type OrderLifecycleEvent struct {
	Venue         Venue
	Symbol        string
	ClientOrderID string
	VenueOrderID  string
	State         string
	FilledQty     float64
	AvgPrice      float64
	SourceTSMS    int64
	IngestTSMS    int64
	EmitTSMS      int64
}

type OrderIntent struct {
	IntentID    string
	StrategyID  string
	Symbol      string
	Side        string
	Price       float64
	Quantity    float64
	TimeInForce string
}

type RiskDecisionType string

const (
	RiskDecisionAllow  RiskDecisionType = "allow"
	RiskDecisionReject RiskDecisionType = "reject"
)

type RiskDecision struct {
	Intent      OrderIntent
	Decision    RiskDecisionType
	RuleID      string
	ReasonCode  string
	EvaluatedMS int64
}

type OrderState string

const (
	OrderStateNew      OrderState = "new"
	OrderStateAck      OrderState = "ack"
	OrderStatePartial  OrderState = "partial_filled"
	OrderStateFilled   OrderState = "filled"
	OrderStateCanceled OrderState = "canceled"
	OrderStateRejected OrderState = "rejected"
)

type OrderEvent struct {
	EventID       string
	ClientOrderID string
	VenueOrderID  string
	Symbol        string
	State         OrderState
	FilledQty     float64
	AvgPrice      float64
}

type Order struct {
	ClientOrderID string
	VenueOrderID  string
	Symbol        string
	State         OrderState
	FilledQty     float64
	AvgPrice      float64
	StateVersion  int64
	UpdatedMS     int64
}

type TradeFillEvent struct {
	TradeID    string
	AccountID  string
	Symbol     string
	Side       string
	FillQty    float64
	FillPrice  float64
	Fee        float64
	SourceTSMS int64
}

type PositionSnapshot struct {
	AccountID   string
	Symbol      string
	Quantity    float64
	AvgCost     float64
	RealizedPnL float64
	UpdatedMS   int64
}
