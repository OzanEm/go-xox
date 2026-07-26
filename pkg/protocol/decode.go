package protocol

import "encoding/json"

func DecodePayload[T any](e Envelope) (T, error) {
	var out T
	if len(e.Payload) == 0 {
		return out, nil
	}
	err := json.Unmarshal(e.Payload, &out)
	if err != nil {
		return out, err
	}
	return out, nil
}
