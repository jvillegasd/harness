package skills

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
)

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
	o.keys = slices.DeleteFunc(o.keys, func(k string) bool { return k == key })
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

func (lf *lockFile) MarshalJSON() ([]byte, error) {
	raw, err := lf.skills.MarshalJSON()
	if err != nil {
		return nil, err
	}
	lf.doc.set("skills", raw)
	return lf.doc.MarshalJSON()
}

func writeNoEscape(buf *bytes.Buffer, v any) error {
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return err
	}
	buf.Truncate(buf.Len() - 1)
	return nil
}

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
