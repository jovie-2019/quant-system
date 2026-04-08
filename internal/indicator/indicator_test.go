package indicator

import (
	"math"
	"testing"
)

const epsilon = 1e-9

func almostEqual(a, b, tol float64) bool {
	return math.Abs(a-b) < tol
}

func TestSMA(t *testing.T) {
	input := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	expected := []float64{0, 0, 2, 3, 4, 5, 6, 7, 8, 9}

	result := SMA(input, 3)
	if len(result) != len(expected) {
		t.Fatalf("SMA length mismatch: got %d, want %d", len(result), len(expected))
	}
	for i, v := range expected {
		if !almostEqual(result[i], v, epsilon) {
			t.Errorf("SMA[%d] = %f, want %f", i, result[i], v)
		}
	}
}

func TestEMA(t *testing.T) {
	input := []float64{10, 11, 12, 13, 14, 15}
	period := 3

	result := EMA(input, period)
	if len(result) != len(input) {
		t.Fatalf("EMA length mismatch: got %d, want %d", len(result), len(input))
	}

	// First non-zero value should equal SMA of first 3 values.
	expectedSeed := (10.0 + 11.0 + 12.0) / 3.0
	if !almostEqual(result[2], expectedSeed, epsilon) {
		t.Errorf("EMA seed = %f, want %f", result[2], expectedSeed)
	}

	// Verify subsequent values follow EMA formula: EMA = (close - prevEMA) * k + prevEMA
	k := 2.0 / float64(period+1)
	for i := 3; i < len(input); i++ {
		expected := (input[i]-result[i-1])*k + result[i-1]
		if !almostEqual(result[i], expected, epsilon) {
			t.Errorf("EMA[%d] = %f, want %f", i, result[i], expected)
		}
	}
}

func TestRSI(t *testing.T) {
	input := []float64{
		44, 44.34, 44.09, 43.61, 44.33, 44.83, 45.10, 45.42, 45.84,
		46.08, 45.89, 46.03, 45.61, 46.28, 46.28, 46.00, 46.03, 46.41,
		46.22, 46.21,
	}
	period := 14

	result := RSI(input, period)
	if len(result) != len(input) {
		t.Fatalf("RSI length mismatch: got %d, want %d", len(result), len(input))
	}

	// First `period` values should be 0.
	for i := 0; i < period; i++ {
		if result[i] != 0 {
			t.Errorf("RSI[%d] = %f, want 0", i, result[i])
		}
	}

	// Last value should be approximately 68.4 (Wilder's smoothing on this dataset).
	lastRSI := result[len(result)-1]
	if !almostEqual(lastRSI, 68.356, 0.5) {
		t.Errorf("RSI last value = %f, want approximately 68.4", lastRSI)
	}
}

func TestMACD(t *testing.T) {
	// Generate 35 data points.
	input := make([]float64, 35)
	for i := range input {
		input[i] = 100 + float64(i)*0.5 + 3*math.Sin(float64(i)*0.3)
	}

	result := MACD(input, 12, 26, 9)
	if len(result.MACD) != len(input) {
		t.Fatalf("MACD length mismatch: got %d, want %d", len(result.MACD), len(input))
	}

	// Verify MACD = fastEMA - slowEMA.
	fastEMA := EMA(input, 12)
	slowEMA := EMA(input, 26)
	for i := 0; i < len(input); i++ {
		expected := fastEMA[i] - slowEMA[i]
		if !almostEqual(result.MACD[i], expected, epsilon) {
			t.Errorf("MACD[%d] = %f, want %f", i, result.MACD[i], expected)
		}
	}

	// Verify histogram = MACD - Signal where signal is non-zero.
	for i := 0; i < len(input); i++ {
		if result.Signal[i] != 0 || result.Histogram[i] != 0 {
			expected := result.MACD[i] - result.Signal[i]
			if !almostEqual(result.Histogram[i], expected, epsilon) {
				t.Errorf("Histogram[%d] = %f, want %f", i, result.Histogram[i], expected)
			}
		}
	}
}

func TestBollinger(t *testing.T) {
	input := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	period := 5
	stddev := 2.0

	result := Bollinger(input, period, stddev)

	// Verify middle = SMA(5).
	sma := SMA(input, period)
	for i := 0; i < len(input); i++ {
		if !almostEqual(result.Middle[i], sma[i], epsilon) {
			t.Errorf("Middle[%d] = %f, want %f", i, result.Middle[i], sma[i])
		}
	}

	// Verify upper > middle > lower for valid indices.
	for i := period - 1; i < len(input); i++ {
		if result.Upper[i] <= result.Middle[i] {
			t.Errorf("Upper[%d] (%f) should be > Middle[%d] (%f)", i, result.Upper[i], i, result.Middle[i])
		}
		if result.Lower[i] >= result.Middle[i] {
			t.Errorf("Lower[%d] (%f) should be < Middle[%d] (%f)", i, result.Lower[i], i, result.Middle[i])
		}
	}

	// Verify band width is consistent (stddev of [1..5] == stddev of [2..6] etc for linear data).
	for i := period; i < len(input); i++ {
		widthPrev := result.Upper[i-1] - result.Lower[i-1]
		widthCurr := result.Upper[i] - result.Lower[i]
		if !almostEqual(widthPrev, widthCurr, 1e-6) {
			t.Errorf("Band width changed: [%d]=%f, [%d]=%f", i-1, widthPrev, i, widthCurr)
		}
	}
}

func TestEmptyInput(t *testing.T) {
	empty := []float64{}

	if r := SMA(empty, 3); len(r) != 0 {
		t.Errorf("SMA empty: got len %d, want 0", len(r))
	}
	if r := EMA(empty, 3); len(r) != 0 {
		t.Errorf("EMA empty: got len %d, want 0", len(r))
	}
	if r := RSI(empty, 14); len(r) != 0 {
		t.Errorf("RSI empty: got len %d, want 0", len(r))
	}

	macdR := MACD(empty, 12, 26, 9)
	if len(macdR.MACD) != 0 {
		t.Errorf("MACD empty: got len %d, want 0", len(macdR.MACD))
	}

	bollR := Bollinger(empty, 20, 2.0)
	if len(bollR.Middle) != 0 {
		t.Errorf("Bollinger empty: got len %d, want 0", len(bollR.Middle))
	}
}

func TestPeriodLargerThanInput(t *testing.T) {
	input := []float64{1, 2, 3}
	period := 10

	smaR := SMA(input, period)
	for i, v := range smaR {
		if v != 0 {
			t.Errorf("SMA[%d] = %f, want 0", i, v)
		}
	}

	emaR := EMA(input, period)
	for i, v := range emaR {
		if v != 0 {
			t.Errorf("EMA[%d] = %f, want 0", i, v)
		}
	}

	rsiR := RSI(input, period)
	for i, v := range rsiR {
		if v != 0 {
			t.Errorf("RSI[%d] = %f, want 0", i, v)
		}
	}

	bollR := Bollinger(input, period, 2.0)
	for i, v := range bollR.Middle {
		if v != 0 {
			t.Errorf("Bollinger.Middle[%d] = %f, want 0", i, v)
		}
	}
}
