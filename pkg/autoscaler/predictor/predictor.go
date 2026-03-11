package predictor

import (
	"math"
)

// Predictor predicts a future value from a time series (for pre-scaling).
type Predictor struct {
	Method string // "linear" or "exponential"
	Alpha  float64 // for exponential smoothing (0 < alpha <= 1)
}

// NewPredictor creates a predictor. Method "exponential" uses alpha=0.3 by default.
func NewPredictor(method string, alpha float64) *Predictor {
	if alpha <= 0 || alpha > 1 {
		alpha = 0.3
	}
	return &Predictor{Method: method, Alpha: alpha}
}

// Predict returns the predicted value at "now" (or slightly in the future) from the series.
// For "exponential": single exponential smoothing, result is weighted toward recent values.
// For "linear": simple linear regression over the series, then extrapolate one step.
func (p *Predictor) Predict(series []float64) float64 {
	if len(series) == 0 {
		return 0
	}
	if len(series) == 1 {
		return series[0]
	}
	switch p.Method {
	case "linear":
		return p.linear(series)
	case "exponential":
		return p.exponential(series)
	default:
		return p.exponential(series)
	}
}

func (p *Predictor) exponential(series []float64) float64 {
	s := series[0]
	for i := 1; i < len(series); i++ {
		s = p.Alpha*series[i] + (1-p.Alpha)*s
	}
	return s
}

// linear performs least-squares linear regression and returns the predicted next value.
func (p *Predictor) linear(series []float64) float64 {
	n := float64(len(series))
	var sumX, sumY, sumXY, sumX2 float64
	for i, y := range series {
		x := float64(i)
		sumX += x
		sumY += y
		sumXY += x * y
		sumX2 += x * x
	}
	denom := n*sumX2 - sumX*sumX
	if math.Abs(denom) < 1e-10 {
		return series[len(series)-1]
	}
	slope := (n*sumXY - sumX*sumY) / denom
	intercept := (sumY - slope*sumX) / n
	// predict one step ahead
	return intercept + slope*n
}
