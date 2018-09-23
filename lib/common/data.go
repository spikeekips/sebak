package common

import "encoding/json"

func EncodeJSONValue(v interface{}) (b []byte, err error) {
	if b, err = json.Marshal(v); err != nil {
		return
	}

	return
}

func PrettyJSONValue(v interface{}) string {
	if b, err := json.MarshalIndent(v, "", "  "); err != nil {
		return ""
	} else {
		return string(b)
	}
}

func DecodeJSONValue(b []byte, v interface{}) (err error) {
	if err = json.Unmarshal(b, v); err != nil {
		return
	}
	return
}

type Serializable interface {
	Serialize() ([]byte, error)
}
