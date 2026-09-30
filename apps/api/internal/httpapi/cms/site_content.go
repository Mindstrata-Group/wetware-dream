package cms

import "encoding/json"

func CloneSiteContent(src map[string]json.RawMessage) map[string]json.RawMessage {
	dst := make(map[string]json.RawMessage, len(src))
	for k, v := range src {
		dst[k] = append(json.RawMessage(nil), v...)
	}
	return dst
}
