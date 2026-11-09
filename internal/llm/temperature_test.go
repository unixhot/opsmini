package llm

import (
	"encoding/json"
	"testing"
)

// A nil temperature must disappear from the wire body entirely: relays that
// validate strictly reject an unexpected key, and providers would otherwise
// treat "we never configured this" as "make it deterministic".
func TestTemperatureOmittedWhenUnset(t *testing.T) {
	body, err := json.Marshal(oaRequest{Model: "m"})
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	_ = json.Unmarshal(body, &m)
	if _, ok := m["temperature"]; ok {
		t.Fatalf("temperature must be absent, got %v", m["temperature"])
	}
}

// Zero is a real value and must survive serialisation.
func TestTemperatureZeroIsSent(t *testing.T) {
	zero := 0.0
	body, _ := json.Marshal(oaRequest{Model: "m", Temperature: &zero})
	var m map[string]any
	_ = json.Unmarshal(body, &m)
	v, ok := m["temperature"]
	if !ok {
		t.Fatal("temperature=0 was dropped")
	}
	if v.(float64) != 0 {
		t.Fatalf("got %v", v)
	}
}

func TestTemperatureSentForAnthropicAndGemini(t *testing.T) {
	half := 0.5
	ab, _ := json.Marshal(antRequest{Model: "m", MaxTokens: 10, Temperature: &half})
	var am map[string]any
	_ = json.Unmarshal(ab, &am)
	if am["temperature"] != 0.5 {
		t.Fatalf("anthropic: %v", am["temperature"])
	}

	gb, _ := json.Marshal(gemRequest{GenerationConfig: &gemGenCfg{Temperature: &half}})
	var gm map[string]any
	_ = json.Unmarshal(gb, &gm)
	gen, _ := gm["generationConfig"].(map[string]any)
	if gen["temperature"] != 0.5 {
		t.Fatalf("gemini: %v", gen["temperature"])
	}
}
