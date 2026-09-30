package httpapi

import "strings"

// Estimated request cost in rubles for direct providers (gemini,
// anthropic): unlike vsegpt they do not return cost in the response,
// so we compute it from the price list. The exchange rate is a fixed constant: the
// goal is not accounting precision but comparability with vsegpt's ruble figures
// in message_usage/statistics.
const aiUSDToRUB = 90.0

// aiModelPrice is a model's price, rubles per 1M tokens.
type aiModelPrice struct {
	In  float64
	Out float64
}

// Matched by a substring of the model name; more specific keys come first
// (flash-lite before flash). An unknown model -> cost 0 (as with vsegpt
// without cost in the response).
var aiPriceTable = map[string][]struct {
	Substr string
	Price  aiModelPrice
}{
	aiProviderAnthropic: {
		{"haiku", aiModelPrice{In: 1.0 * aiUSDToRUB, Out: 5.0 * aiUSDToRUB}},
		{"sonnet", aiModelPrice{In: 2.0 * aiUSDToRUB, Out: 10.0 * aiUSDToRUB}},
		{"opus", aiModelPrice{In: 5.0 * aiUSDToRUB, Out: 25.0 * aiUSDToRUB}},
	},
	aiProviderGemini: {
		{"flash-lite", aiModelPrice{In: 0.25 * aiUSDToRUB, Out: 1.5 * aiUSDToRUB}},
		{"flash", aiModelPrice{In: 1.5 * aiUSDToRUB, Out: 9.0 * aiUSDToRUB}},
		{"pro", aiModelPrice{In: 2.0 * aiUSDToRUB, Out: 12.0 * aiUSDToRUB}},
	},
}

func lookupAIModelPrice(provider, model string) (aiModelPrice, bool) {
	lower := strings.ToLower(model)
	for _, entry := range aiPriceTable[provider] {
		if strings.Contains(lower, entry.Substr) {
			return entry.Price, true
		}
	}
	return aiModelPrice{}, false
}

// estimateAICostRUB returns the request cost in rubles.
// in: regular (uncached) input tokens; cacheRead: tokens read from the
// cache (0.1x the input price); cacheWrite: tokens written to the cache (1.25x at Anthropic;
// Gemini's implicit cache does not bill writes, pass 0).
func estimateAICostRUB(provider, model string, in, out, cacheRead, cacheWrite int) float64 {
	price, ok := lookupAIModelPrice(provider, model)
	if !ok {
		return 0
	}
	perTokIn := price.In / 1e6
	perTokOut := price.Out / 1e6
	cost := float64(in)*perTokIn +
		float64(cacheRead)*perTokIn*0.1 +
		float64(cacheWrite)*perTokIn*1.25 +
		float64(out)*perTokOut
	return cost
}
