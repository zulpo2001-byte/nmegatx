package admin

import "encoding/json"

// marshalMapBool 将 map[string]bool 序列化为 JSON 字符串
func marshalMapBool(m map[string]bool) (string, error) {
	b, err := json.Marshal(m)
	if err != nil {
		return "{}", err
	}
	return string(b), nil
}
