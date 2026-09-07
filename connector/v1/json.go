package v1

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"time"
)

var timestampPattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$`)

func (s *Snapshot) UnmarshalJSON(data []byte) error {
	type wire Snapshot
	if err := checkWireValue(data, reflect.TypeOf(wire{}), "snapshot"); err != nil {
		return err
	}
	var value wire
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*s = Snapshot(value)
	return nil
}

func checkWireValue(data json.RawMessage, typ reflect.Type, path string) error {
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return fmt.Errorf("%s must not be null", path)
	}
	if typ == reflect.TypeOf(time.Time{}) {
		var value string
		if err := json.Unmarshal(data, &value); err != nil {
			return err
		}
		if !timestampPattern.MatchString(value) {
			return fmt.Errorf("%s must be RFC3339 with timezone", path)
		}
		return nil
	}
	switch typ.Kind() {
	case reflect.Struct:
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(data, &fields); err != nil {
			return err
		}
		for i := 0; i < typ.NumField(); i++ {
			field := typ.Field(i)
			tag := strings.Split(field.Tag.Get("json"), ",")
			name := tag[0]
			value, exists := fields[name]
			if !exists {
				if len(tag) == 1 {
					return fmt.Errorf("%s.%s is required", path, name)
				}
				continue
			}
			if err := checkWireValue(value, field.Type, path+"."+name); err != nil {
				return err
			}
		}
	case reflect.Slice:
		var values []json.RawMessage
		if err := json.Unmarshal(data, &values); err != nil {
			return err
		}
		for i, value := range values {
			if err := checkWireValue(value, typ.Elem(), fmt.Sprintf("%s[%d]", path, i)); err != nil {
				return err
			}
		}
	case reflect.Map:
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(data, &fields); err != nil {
			return err
		}
		for name, value := range fields {
			if err := checkWireValue(value, typ.Elem(), path+"."+name); err != nil {
				return err
			}
		}
	default:
		if err := json.Unmarshal(data, reflect.New(typ).Interface()); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
	}
	return nil
}
