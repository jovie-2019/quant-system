package adminapi

import (
	"encoding/json"
	"testing"

	"quant-system/pkg/contracts"
)

func TestValidateParamsRequest(t *testing.T) {
	cases := []struct {
		name string
		req  StrategyParamsRequest
		ok   bool
	}{
		{"missing type", StrategyParamsRequest{}, false},
		{"unknown type", StrategyParamsRequest{Type: "weird"}, false},
		{"update without params", StrategyParamsRequest{Type: contracts.StrategyControlUpdateParams}, false},
		{
			"update with non-object params",
			StrategyParamsRequest{
				Type:   contracts.StrategyControlUpdateParams,
				Params: json.RawMessage(`[1,2,3]`),
			},
			false,
		},
		{
			"update happy path",
			StrategyParamsRequest{
				Type:   contracts.StrategyControlUpdateParams,
				Params: json.RawMessage(`{"window_size":10}`),
			},
			true,
		},
		{"pause ok", StrategyParamsRequest{Type: contracts.StrategyControlPause}, true},
		{"resume ok", StrategyParamsRequest{Type: contracts.StrategyControlResume}, true},
		{"shadow_on ok", StrategyParamsRequest{Type: contracts.StrategyControlShadowOn}, true},
		{"shadow_off ok", StrategyParamsRequest{Type: contracts.StrategyControlShadowOff}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateParamsRequest(tc.req)
			if (err == nil) != tc.ok {
				t.Fatalf("err=%v ok=%v", err, tc.ok)
			}
		})
	}
}

func TestParseTrailingID(t *testing.T) {
	cases := []struct {
		path, prefix, suffix string
		want                 int64
		wantErr              bool
	}{
		{"/api/v1/strategies/42/params", "/api/v1/strategies/", "/params", 42, false},
		{"/api/v1/strategies/7/revisions", "/api/v1/strategies/", "/revisions", 7, false},
		{"/api/v1/strategies/notanid/params", "/api/v1/strategies/", "/params", 0, true},
		{"/wrong-prefix/42/params", "/api/v1/strategies/", "/params", 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			got, err := parseTrailingID(tc.path, tc.prefix, tc.suffix)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err=%v wantErr=%v", err, tc.wantErr)
			}
			if !tc.wantErr && got != tc.want {
				t.Fatalf("got=%d want=%d", got, tc.want)
			}
		})
	}
}
