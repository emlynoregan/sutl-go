package sutl

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// object is a MLSNBN map that remembers JSON key order so path wildcards
// match the frozen Studio fixtures.
type object struct {
	keys []string
	m    map[string]Value
}

// DecodeJSON reads a MLSNBN value, preserving object key order.
func DecodeJSON(data []byte) (Value, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	return decodeJSONValue(decoder)
}

func decodeJSONValue(decoder *json.Decoder) (Value, error) {
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	switch t := token.(type) {
	case json.Delim:
		switch t {
		case '{':
			return decodeJSONObject(decoder)
		case '[':
			return decodeJSONArray(decoder)
		default:
			return nil, fmt.Errorf("unexpected delimiter %v", t)
		}
	case bool, string:
		return t, nil
	case nil:
		return nil, nil
	case json.Number:
		if i, err := t.Int64(); err == nil {
			return int(i), nil
		}
		f, err := t.Float64()
		if err != nil {
			return nil, err
		}
		return f, nil
	default:
		return nil, fmt.Errorf("unexpected JSON token %T", token)
	}
}

func decodeJSONObject(decoder *json.Decoder) (Value, error) {
	obj := &object{m: map[string]Value{}}
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		key, ok := token.(string)
		if !ok {
			return nil, fmt.Errorf("expected object key, got %T", token)
		}
		value, err := decodeJSONValue(decoder)
		if err != nil {
			return nil, err
		}
		if _, exists := obj.m[key]; !exists {
			obj.keys = append(obj.keys, key)
		}
		obj.m[key] = value
	}
	end, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	if end != json.Delim('}') {
		return nil, fmt.Errorf("expected end of object, got %v", end)
	}
	return obj, nil
}

func decodeJSONArray(decoder *json.Decoder) (Value, error) {
	var list []Value
	for decoder.More() {
		value, err := decodeJSONValue(decoder)
		if err != nil {
			return nil, err
		}
		list = append(list, value)
	}
	end, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	if end != json.Delim(']') {
		return nil, fmt.Errorf("expected end of array, got %v", end)
	}
	if list == nil {
		list = []Value{}
	}
	return list, nil
}

func objectValues(value Value) []Value {
	if obj, ok := value.(*object); ok {
		out := make([]Value, 0, len(obj.keys))
		for _, key := range obj.keys {
			out = append(out, obj.m[key])
		}
		return out
	}
	if m, ok := asMap(value); ok {
		keys := sortedKeys(m)
		out := make([]Value, 0, len(keys))
		for _, key := range keys {
			out = append(out, m[key])
		}
		return out
	}
	return nil
}

func (o *object) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, key := range o.keys {
		if i > 0 {
			buf.WriteByte(',')
		}
		keyBytes, err := json.Marshal(key)
		if err != nil {
			return nil, err
		}
		buf.Write(keyBytes)
		buf.WriteByte(':')
		valueBytes, err := json.Marshal(o.m[key])
		if err != nil {
			return nil, err
		}
		buf.Write(valueBytes)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}
