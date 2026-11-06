package telemetry

import "github.com/zaansaeed/k8s-reconcile/internal/llm"

type price struct {
	input      float64
	output     float64
	cacheRead  float64
	cacheWrite float64
}

// TODO(zaan): cacheWrite is input x 1.25 (the 5 minute TTL rate); confirm every
// row against the published pricing page before any number goes in the writeup.
var prices = map[string]price{
	llm.ModelOpus:   {input: 4.00, output: 20.00, cacheRead: 0.20, cacheWrite: 5.00},
	llm.ModelSonnet: {input: 2.00, output: 10.00, cacheRead: 0.20, cacheWrite: 2.50},
	llm.ModelHaiku:  {input: 0.10, output: 0.50, cacheRead: 0.01, cacheWrite: 0.125},
}

const tokensPerMillion = 1_000_000.0

func CostUSD(model string, usage llm.Usage) (float64, bool) {
	rate, known := prices[model]
	if !known {
		return 0, false
	}
	cost := float64(usage.InputTokens)*rate.input +
		float64(usage.OutputTokens)*rate.output +
		float64(usage.CacheReadTokens)*rate.cacheRead +
		float64(usage.CacheWriteTokens)*rate.cacheWrite
	return cost / tokensPerMillion, true
}

type Run struct {
	Model string
	Steps int
	Usage llm.Usage
}

func (run *Run) Record(usage llm.Usage) {
	run.Steps++
	run.Usage.Add(usage)
}

func (run *Run) CostUSD() (float64, bool) {
	return CostUSD(run.Model, run.Usage)
}
