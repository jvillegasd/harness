package skills

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

// orderedObject preserves a JSON object's keys and their raw values in file order, so a rewrite re-encodes only what it must and drops no field.
type orderedObject struct {
	keys   []string
	values map[string]json.RawMessage
}

func (o *orderedObject) UnmarshalJSON(data []byte) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	if delim, ok := tok.(json.Delim); !ok || delim != '{' {
		return errors.New("not a JSON object")
	}
	o.values = map[string]json.RawMessage{}
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return err
		}
		key, ok := keyTok.(string)
		if !ok {
			return errors.New("non-string key")
		}
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return err
		}
		o.set(key, raw)
	}
	return nil
}

func (o *orderedObject) set(key string, raw json.RawMessage) {
	if _, ok := o.values[key]; !ok {
		o.keys = append(o.keys, key)
	}
	o.values[key] = raw
}

func (o *orderedObject) has(key string) bool {
	_, ok := o.values[key]
	return ok
}

func (o *orderedObject) del(key string) {
	if !o.has(key) {
		return
	}
	delete(o.values, key)
	keys := o.keys[:0]
	for _, k := range o.keys {
		if k != key {
			keys = append(keys, k)
		}
	}
	o.keys = keys
}

func (o *orderedObject) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, key := range o.keys {
		if i > 0 {
			buf.WriteByte(',')
		}
		if err := writeNoEscape(&buf, key); err != nil {
			return nil, err
		}
		buf.WriteByte(':')
		buf.Write(o.values[key])
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// lockFile is the skills-lock.json document: an ordered top level plus the skills object parsed from it.
type lockFile struct {
	doc    orderedObject
	skills orderedObject
}

func (lf *lockFile) UnmarshalJSON(data []byte) error {
	if err := json.Unmarshal(data, &lf.doc); err != nil {
		return err
	}
	raw, ok := lf.doc.values["skills"]
	if !ok {
		raw = json.RawMessage("{}")
		lf.doc.set("skills", raw)
	}
	if err := json.Unmarshal(raw, &lf.skills); err != nil {
		return fmt.Errorf("skills: %w", err)
	}
	return nil
}

// MarshalJSON re-encodes only skills and keeps every other key and value raw, in file order, without HTML escaping, so writeLock stays byte-identical to skills.sh's JSON.stringify output.
func (lf *lockFile) MarshalJSON() ([]byte, error) {
	raw, err := lf.skills.MarshalJSON()
	if err != nil {
		return nil, err
	}
	lf.doc.set("skills", raw)
	return lf.doc.MarshalJSON()
}

// writeNoEscape encodes v without HTML escaping, matching JSON.stringify, and drops the newline Encode appends.
func writeNoEscape(buf *bytes.Buffer, v any) error {
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return err
	}
	buf.Truncate(buf.Len() - 1)
	return nil
}

// entry is the subset of a lock entry verify reads.
type entry struct {
	ComputedHash string `json:"computedHash"`
}

func readLock(path string) (*lockFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var lf lockFile
	if err := json.Unmarshal(data, &lf); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &lf, nil
}

// writeLock reproduces skills.sh's canonical output: two-space indent, no HTML escaping, trailing newline, keys sorted.
func writeLock(path string, lf *lockFile) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(lf); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}
