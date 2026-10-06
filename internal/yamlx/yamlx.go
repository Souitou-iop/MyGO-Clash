// Package yamlx is an order-preserving tree for Clash configurations.
//
// Profiles are YAML documents that users read and edit, so the generated
// runtime configuration keeps the order of their keys instead of sorting them
// as map[string]any would. A tree is made of *Map (ordered mappings), []any
// (sequences) and scalars: string, bool, nil, int, int64, uint64 and float64.
// Anchors, aliases and merge keys (<<) are resolved while parsing.
package yamlx

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"slices"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Map is a mapping with string keys that remembers their order.
type Map struct {
	keys []string
	vals map[string]any
}

// NewMap returns an empty map.
func NewMap() *Map { return &Map{vals: map[string]any{}} }

// MapOf builds a map from alternating keys and values.
func MapOf(kv ...any) *Map {
	m := NewMap()
	for i := 0; i+1 < len(kv); i += 2 {
		m.Set(kv[i].(string), kv[i+1])
	}
	return m
}

// Len returns the number of keys.
func (m *Map) Len() int {
	if m == nil {
		return 0
	}
	return len(m.keys)
}

// Keys returns the keys in order. The slice must not be modified.
func (m *Map) Keys() []string {
	if m == nil {
		return nil
	}
	return m.keys
}

// Has reports whether the key is present.
func (m *Map) Has(k string) bool {
	if m == nil {
		return false
	}
	_, ok := m.vals[k]
	return ok
}

// Get returns the value of a key.
func (m *Map) Get(k string) (any, bool) {
	if m == nil {
		return nil, false
	}
	v, ok := m.vals[k]
	return v, ok
}

// Value returns the value of a key, or nil.
func (m *Map) Value(k string) any {
	v, _ := m.Get(k)
	return v
}

// Set sets a key, keeping its position when it exists already.
func (m *Map) Set(k string, v any) {
	if m.vals == nil {
		m.vals = map[string]any{}
	}
	if _, ok := m.vals[k]; !ok {
		m.keys = append(m.keys, k)
	}
	m.vals[k] = v
}

// SetFirst sets a key and moves it to the front.
func (m *Map) SetFirst(k string, v any) {
	m.Delete(k)
	if m.vals == nil {
		m.vals = map[string]any{}
	}
	m.keys = append([]string{k}, m.keys...)
	m.vals[k] = v
}

// Delete removes a key.
func (m *Map) Delete(k string) {
	if m == nil {
		return
	}
	if _, ok := m.vals[k]; !ok {
		return
	}
	delete(m.vals, k)
	if i := slices.Index(m.keys, k); i >= 0 {
		m.keys = slices.Delete(m.keys, i, i+1)
	}
}

// Range calls fn for each key in order until it returns false.
func (m *Map) Range(fn func(k string, v any) bool) {
	if m == nil {
		return
	}
	for _, k := range m.keys {
		if !fn(k, m.vals[k]) {
			return
		}
	}
}

// Map returns the mapping under a key, or nil.
func (m *Map) Map(k string) *Map {
	v, _ := m.Value(k).(*Map)
	return v
}

// EnsureMap returns the mapping under a key, creating it when missing or of
// another type.
func (m *Map) EnsureMap(k string) *Map {
	if v, ok := m.Value(k).(*Map); ok {
		return v
	}
	v := NewMap()
	m.Set(k, v)
	return v
}

// Slice returns the sequence under a key, or nil.
func (m *Map) Slice(k string) []any {
	v, _ := m.Value(k).([]any)
	return v
}

// String returns the string under a key, or "".
func (m *Map) String(k string) string {
	switch v := m.Value(k).(type) {
	case string:
		return v
	case nil:
		return ""
	default:
		return fmt.Sprint(v)
	}
}

// Bool returns the boolean under a key and whether it is one.
func (m *Map) Bool(k string) (bool, bool) {
	v, ok := m.Value(k).(bool)
	return v, ok
}

// Int returns the integer under a key and whether it is one.
func (m *Map) Int(k string) (int64, bool) { return ToInt(m.Value(k)) }

// Clone returns a deep copy.
func (m *Map) Clone() *Map {
	if m == nil {
		return nil
	}
	c := &Map{keys: slices.Clone(m.keys), vals: make(map[string]any, len(m.vals))}
	for k, v := range m.vals {
		c.vals[k] = CloneValue(v)
	}
	return c
}

// ToStd converts the map into a map[string]any tree, for code that does not
// care about order.
func (m *Map) ToStd() map[string]any {
	out := make(map[string]any, m.Len())
	m.Range(func(k string, v any) bool {
		out[k] = ToStd(v)
		return true
	})
	return out
}

// ToStd converts a tree into map[string]any and []any values.
func ToStd(v any) any {
	switch v := v.(type) {
	case *Map:
		return v.ToStd()
	case []any:
		out := make([]any, len(v))
		for i, e := range v {
			out[i] = ToStd(e)
		}
		return out
	default:
		return v
	}
}

// FromStd converts map[string]any trees into *Map trees, sorting keys of
// plain maps so that the result is deterministic.
func FromStd(v any) any {
	switch v := v.(type) {
	case *Map:
		return v
	case map[string]any:
		m := NewMap()
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		slices.Sort(keys)
		for _, k := range keys {
			m.Set(k, FromStd(v[k]))
		}
		return m
	case []any:
		out := make([]any, len(v))
		for i, e := range v {
			out[i] = FromStd(e)
		}
		return out
	case []string:
		out := make([]any, len(v))
		for i, e := range v {
			out[i] = e
		}
		return out
	case int:
		return int64(v)
	default:
		return v
	}
}

// CloneValue deep copies a tree value.
func CloneValue(v any) any {
	switch v := v.(type) {
	case *Map:
		return v.Clone()
	case []any:
		out := make([]any, len(v))
		for i, e := range v {
			out[i] = CloneValue(e)
		}
		return out
	default:
		return v
	}
}

// Equal reports whether two tree values are deeply equal. Numbers compare by
// value across their Go types.
func Equal(a, b any) bool {
	switch av := a.(type) {
	case *Map:
		bv, ok := b.(*Map)
		if !ok || av.Len() != bv.Len() {
			return false
		}
		for _, k := range av.keys {
			bvv, ok := bv.Get(k)
			if !ok || !Equal(av.vals[k], bvv) {
				return false
			}
		}
		return true
	case []any:
		bv, ok := b.([]any)
		if !ok || len(av) != len(bv) {
			return false
		}
		for i := range av {
			if !Equal(av[i], bv[i]) {
				return false
			}
		}
		return true
	}
	if an, ok := ToFloat(a); ok {
		bn, ok := ToFloat(b)
		return ok && an == bn
	}
	return a == b
}

// ToInt converts a numeric tree value (or a numeric string) to int64.
func ToInt(v any) (int64, bool) {
	switch v := v.(type) {
	case int:
		return int64(v), true
	case int64:
		return v, true
	case uint64:
		if v > math.MaxInt64 {
			return 0, false
		}
		return int64(v), true
	case float64:
		if v == math.Trunc(v) {
			return int64(v), true
		}
	case string:
		n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		return n, err == nil
	}
	return 0, false
}

// ToFloat converts a numeric tree value to float64.
func ToFloat(v any) (float64, bool) {
	switch v := v.(type) {
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case uint64:
		return float64(v), true
	case float64:
		return v, true
	}
	return 0, false
}

// Parse decodes a YAML document whose top level is a mapping. An empty
// document is an empty map.
func Parse(data []byte) (*Map, error) {
	v, err := ParseValue(data)
	if err != nil {
		return nil, err
	}
	switch v := v.(type) {
	case nil:
		return NewMap(), nil
	case *Map:
		return v, nil
	default:
		return nil, fmt.Errorf("the top level of the document is a %s, not a mapping", typeName(v))
	}
}

// ParseValue decodes a YAML document of any shape.
func ParseValue(data []byte) (any, error) {
	var doc yaml.Node
	dec := yaml.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(&doc); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, nil
		}
		return nil, err
	}
	if doc.Kind == 0 {
		return nil, nil
	}
	c := converter{seen: map[*yaml.Node]bool{}}
	return c.value(&doc)
}

type converter struct {
	seen  map[*yaml.Node]bool // aliases being expanded, to stop cycles
	nodes int
}

const maxNodes = 4 << 20 // bounds alias expansion ("billion laughs")

func (c *converter) value(n *yaml.Node) (any, error) {
	c.nodes++
	if c.nodes > maxNodes {
		return nil, errors.New("the document expands to too many nodes")
	}
	switch n.Kind {
	case yaml.DocumentNode:
		if len(n.Content) == 0 {
			return nil, nil
		}
		return c.value(n.Content[0])
	case yaml.AliasNode:
		if c.seen[n.Alias] {
			return nil, fmt.Errorf("line %d: recursive alias", n.Line)
		}
		c.seen[n.Alias] = true
		defer delete(c.seen, n.Alias)
		return c.value(n.Alias)
	case yaml.SequenceNode:
		out := make([]any, 0, len(n.Content))
		for _, e := range n.Content {
			v, err := c.value(e)
			if err != nil {
				return nil, err
			}
			out = append(out, v)
		}
		return out, nil
	case yaml.MappingNode:
		return c.mapping(n)
	case yaml.ScalarNode:
		return scalar(n)
	}
	return nil, fmt.Errorf("line %d: unsupported YAML node", n.Line)
}

func (c *converter) mapping(n *yaml.Node) (*Map, error) {
	m := NewMap()
	var merges []*Map
	for i := 0; i+1 < len(n.Content); i += 2 {
		kn, vn := n.Content[i], n.Content[i+1]
		if kn.Kind == yaml.ScalarNode && kn.Value == "<<" && kn.Tag != "!!str" {
			v, err := c.value(vn)
			if err != nil {
				return nil, err
			}
			switch v := v.(type) {
			case *Map:
				merges = append(merges, v)
			case []any:
				for _, e := range v {
					if em, ok := e.(*Map); ok {
						merges = append(merges, em)
					}
				}
			default:
				return nil, fmt.Errorf("line %d: a merge key needs a mapping", kn.Line)
			}
			continue
		}
		kv, err := c.value(kn)
		if err != nil {
			return nil, err
		}
		key := keyString(kv)
		v, err := c.value(vn)
		if err != nil {
			return nil, err
		}
		m.Set(key, v)
	}
	// Explicit keys win over merged ones; earlier merges win over later ones.
	for _, mm := range merges {
		for _, k := range mm.keys {
			if !m.Has(k) {
				m.Set(k, CloneValue(mm.vals[k]))
			}
		}
	}
	return m, nil
}

func keyString(v any) string {
	switch v := v.(type) {
	case string:
		return v
	case nil:
		return "null"
	default:
		return fmt.Sprint(v)
	}
}

func scalar(n *yaml.Node) (any, error) {
	switch n.ShortTag() {
	case "!!str", "!!binary", "!!timestamp":
		return n.Value, nil
	case "!!null":
		return nil, nil
	case "!!bool":
		var b bool
		if err := n.Decode(&b); err != nil {
			return nil, err
		}
		return b, nil
	case "!!int":
		var i int64
		if err := n.Decode(&i); err == nil {
			return i, nil
		}
		var u uint64
		if err := n.Decode(&u); err == nil {
			return u, nil
		}
		return n.Value, nil
	case "!!float":
		var f float64
		if err := n.Decode(&f); err != nil {
			return nil, err
		}
		return f, nil
	default:
		return n.Value, nil
	}
}

func typeName(v any) string {
	switch v.(type) {
	case []any:
		return "sequence"
	case string:
		return "string"
	case bool:
		return "boolean"
	case nil:
		return "null"
	default:
		return "number"
	}
}

// Marshal encodes a tree value as YAML with two-space indentation.
func Marshal(v any) ([]byte, error) {
	n, err := toNode(v)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(n); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// MarshalYAML implements yaml.Marshaler.
func (m *Map) MarshalYAML() (any, error) { return toNode(m) }

func toNode(v any) (*yaml.Node, error) {
	switch v := v.(type) {
	case *Map:
		n := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		if v == nil {
			return n, nil
		}
		for _, k := range v.keys {
			kn := &yaml.Node{}
			if err := kn.Encode(k); err != nil {
				return nil, err
			}
			vn, err := toNode(v.vals[k])
			if err != nil {
				return nil, err
			}
			n.Content = append(n.Content, kn, vn)
		}
		return n, nil
	case []any:
		n := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		for _, e := range v {
			en, err := toNode(e)
			if err != nil {
				return nil, err
			}
			n.Content = append(n.Content, en)
		}
		return n, nil
	case map[string]any, []string:
		return toNode(FromStd(v))
	default:
		n := &yaml.Node{}
		if err := n.Encode(v); err != nil {
			return nil, err
		}
		return n, nil
	}
}

// MarshalJSON writes the map with its keys in order.
func (m *Map) MarshalJSON() ([]byte, error) {
	if m == nil {
		return []byte("null"), nil
	}
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, k := range m.keys {
		if i > 0 {
			buf.WriteByte(',')
		}
		kb, _ := json.Marshal(k)
		buf.Write(kb)
		buf.WriteByte(':')
		vb, err := json.Marshal(m.vals[k])
		if err != nil {
			return nil, err
		}
		buf.Write(vb)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// UnmarshalJSON reads an object keeping the order of its keys.
func (m *Map) UnmarshalJSON(data []byte) error {
	v, err := DecodeJSON(data)
	if err != nil {
		return err
	}
	mm, ok := v.(*Map)
	if !ok {
		return errors.New("not a JSON object")
	}
	*m = *mm
	return nil
}

// DecodeJSON decodes JSON into a tree, keeping the order of object keys.
// Integral numbers become int64, others float64.
func DecodeJSON(data []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	v, err := decodeJSONValue(dec)
	if err != nil {
		return nil, err
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, errors.New("trailing data after JSON value")
	}
	return v, nil
}

func decodeJSONValue(dec *json.Decoder) (any, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	switch t := tok.(type) {
	case json.Delim:
		switch t {
		case '{':
			m := NewMap()
			for dec.More() {
				kt, err := dec.Token()
				if err != nil {
					return nil, err
				}
				k, ok := kt.(string)
				if !ok {
					return nil, errors.New("object key is not a string")
				}
				v, err := decodeJSONValue(dec)
				if err != nil {
					return nil, err
				}
				m.Set(k, v)
			}
			if _, err := dec.Token(); err != nil {
				return nil, err
			}
			return m, nil
		case '[':
			out := []any{}
			for dec.More() {
				v, err := decodeJSONValue(dec)
				if err != nil {
					return nil, err
				}
				out = append(out, v)
			}
			if _, err := dec.Token(); err != nil {
				return nil, err
			}
			return out, nil
		}
		return nil, fmt.Errorf("unexpected delimiter %v", t)
	case json.Number:
		if i, err := t.Int64(); err == nil {
			return i, nil
		}
		f, err := t.Float64()
		if err != nil {
			return nil, err
		}
		return f, nil
	default:
		return t, nil // string, bool, nil
	}
}
