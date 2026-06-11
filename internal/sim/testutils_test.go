package sim

import "encoding/json"

func msgAsMap(v any) map[string]any {
	if m, ok := v.(map[string]any); ok {
		return m
	}
	b, _ := json.Marshal(v)
	var m map[string]any
	json.Unmarshal(b, &m)
	return m
}
