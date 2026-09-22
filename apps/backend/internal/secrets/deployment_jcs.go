package secrets

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
)

// jcsObject is a strict RFC 8785 object restricted to string and integer
// values. The deployment-secret control protocol uses only those value kinds,
// so the parser never needs to canonicalize floats, nesting, or null.
type jcsObject struct {
	values map[string]jcsValue
	keys   []string
}

type jcsValueKind int

const (
	jcsKindString jcsValueKind = iota
	jcsKindInteger
)

type jcsValue struct {
	kind   jcsValueKind
	text   string
	number int64
}

type jcsField struct {
	key       string
	text      string
	number    int64
	isInteger bool
}

func intJCSField(key string, value int64) jcsField {
	return jcsField{key: key, number: value, isInteger: true}
}

var errJCSNotCanonical = errors.New("payload is not canonical RFC 8785 JSON")

// ParseJCSObject parses a canonical JCS object, rejecting duplicate keys,
// unsupported value kinds, trailing bytes, and any input that does not
// round-trip byte-identically through canonical serialization.
func ParseJCSObject(raw []byte) (*jcsObject, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	object, err := decodeJCSObject(decoder)
	if err != nil {
		return nil, err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return nil, errors.New("payload has trailing bytes")
	}
	if !bytes.Equal(object.Canonical(), raw) {
		return nil, errJCSNotCanonical
	}
	return object, nil
}

func decodeJCSObject(decoder *json.Decoder) (*jcsObject, error) {
	token, err := decoder.Token()
	if err != nil {
		return nil, fmt.Errorf("parse JSON: %w", err)
	}
	if delim, ok := token.(json.Delim); !ok || delim != '{' {
		return nil, errors.New("payload must be a JSON object")
	}
	object := &jcsObject{values: map[string]jcsValue{}}
	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			return nil, fmt.Errorf("parse JSON key: %w", err)
		}
		key, ok := keyToken.(string)
		if !ok {
			return nil, errors.New("payload key must be a string")
		}
		if _, exists := object.values[key]; exists {
			return nil, fmt.Errorf("payload has duplicate key %q", key)
		}
		valueToken, err := decoder.Token()
		if err != nil {
			return nil, fmt.Errorf("parse JSON value: %w", err)
		}
		value, err := jcsValueFromToken(valueToken)
		if err != nil {
			return nil, err
		}
		object.values[key] = value
		object.keys = append(object.keys, key)
	}
	if _, err := decoder.Token(); err != nil {
		return nil, fmt.Errorf("parse JSON object end: %w", err)
	}
	sort.Strings(object.keys)
	return object, nil
}

func jcsValueFromToken(token json.Token) (jcsValue, error) {
	switch value := token.(type) {
	case string:
		return jcsValue{kind: jcsKindString, text: value}, nil
	case json.Number:
		parsed, err := strconv.ParseInt(value.String(), 10, 64)
		if err != nil || strconv.FormatInt(parsed, 10) != value.String() {
			return jcsValue{}, fmt.Errorf("payload value %q is not a canonical integer", value.String())
		}
		return jcsValue{kind: jcsKindInteger, number: parsed}, nil
	default:
		return jcsValue{}, errors.New("payload values must be strings or integers")
	}
}

// newJCSObject builds a canonical object. Later duplicate keys are ignored so
// callers can append a constant field without re-checking their inputs.
func newJCSObject(fields ...jcsField) *jcsObject {
	object := &jcsObject{values: make(map[string]jcsValue, len(fields))}
	for _, field := range fields {
		if _, exists := object.values[field.key]; exists {
			continue
		}
		value := jcsValue{kind: jcsKindString, text: field.text}
		if field.isInteger {
			value = jcsValue{kind: jcsKindInteger, number: field.number}
		}
		object.values[field.key] = value
		object.keys = append(object.keys, field.key)
	}
	sort.Strings(object.keys)
	return object
}

// Canonical serializes the object with sorted keys and no insignificant
// whitespace, matching the byte-identity requirement of the wire protocol.
func (o *jcsObject) Canonical() []byte {
	var buffer bytes.Buffer
	buffer.WriteByte('{')
	for index, key := range o.keys {
		if index > 0 {
			buffer.WriteByte(',')
		}
		buffer.Write(escapeJCSString(key))
		buffer.WriteByte(':')
		value := o.values[key]
		if value.kind == jcsKindInteger {
			buffer.WriteString(strconv.FormatInt(value.number, 10))
			continue
		}
		buffer.Write(escapeJCSString(value.text))
	}
	buffer.WriteByte('}')
	return buffer.Bytes()
}

func (o *jcsObject) Has(key string) bool {
	_, ok := o.values[key]
	return ok
}

func (o *jcsObject) Len() int {
	return len(o.values)
}

func (o *jcsObject) Keys() []string {
	return append([]string(nil), o.keys...)
}

func (o *jcsObject) String(key string) (string, error) {
	value, ok := o.values[key]
	if !ok {
		return "", fmt.Errorf("payload is missing %q", key)
	}
	if value.kind != jcsKindString {
		return "", fmt.Errorf("payload field %q must be a string", key)
	}
	return value.text, nil
}

func (o *jcsObject) Integer(key string) (int64, error) {
	value, ok := o.values[key]
	if !ok {
		return 0, fmt.Errorf("payload is missing %q", key)
	}
	if value.kind != jcsKindInteger {
		return 0, fmt.Errorf("payload field %q must be an integer", key)
	}
	return value.number, nil
}

func escapeJCSString(value string) []byte {
	var buffer bytes.Buffer
	buffer.WriteByte('"')
	for _, char := range value {
		switch char {
		case '"':
			buffer.WriteString(`\"`)
		case '\\':
			buffer.WriteString(`\\`)
		case '\b':
			buffer.WriteString(`\b`)
		case '\t':
			buffer.WriteString(`\t`)
		case '\n':
			buffer.WriteString(`\n`)
		case '\f':
			buffer.WriteString(`\f`)
		case '\r':
			buffer.WriteString(`\r`)
		default:
			if char < 0x20 {
				fmt.Fprintf(&buffer, `\u%04x`, char)
				continue
			}
			buffer.WriteRune(char)
		}
	}
	buffer.WriteByte('"')
	return buffer.Bytes()
}
