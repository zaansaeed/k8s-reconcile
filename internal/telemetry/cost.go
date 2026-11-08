package telemetry

import (
	"fmt"

	"github.com/zaansaeed/k8s-reconcile/internal/llm"
)

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

type Run struct {
	Model string
	Steps int
	Usage llm.Usage
}

func (run *Run) Record(usage llm.Usage) {
	run.Steps++
	run.Usage.Add(usage)
}

// CostUSD prices the run's tokens. It reports false for a model with no price.
func (run *Run) CostUSD() (float64, bool) {
	rate, known := prices[run.Model]
	if !known {
		return 0, false
	}
	u := run.Usage
	cost := float64(u.InputTokens)*rate.input +
		float64(u.OutputTokens)*rate.output +
		float64(u.CacheReadTokens)*rate.cacheRead +
		float64(u.CacheWriteTokens)*rate.cacheWrite
	return cost / tokensPerMillion, true
}

// UsageLine formats token counts, labelling the cache counts with their price
// relative to plain input. The labels come from the price table, so they can't
// disagree with the cost; a model with no price gets no labels.
func (run *Run) UsageLine(u llm.Usage) string {
	rate, known := prices[run.Model]
	if !known {
		return fmt.Sprintf("in %d  out %d  cache_write %d  cache_read %d",
			u.InputTokens, u.OutputTokens, u.CacheWriteTokens, u.CacheReadTokens)
	}
	// %.3g keeps at most 3 significant digits, so 0.01/0.10, which is
	// 0.09999999999999999 in floating point, prints as 0.1.
	return fmt.Sprintf("in %d (uncached 1x)  out %d  cache_write %d (%.3gx)  cache_read %d (%.3gx)",
		u.InputTokens, u.OutputTokens,
		u.CacheWriteTokens, rate.cacheWrite/rate.input,
		u.CacheReadTokens, rate.cacheRead/rate.input)
}
