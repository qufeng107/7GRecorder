package main

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

type embeddedContract struct {
	ID int64 `json:"id"`
}

func TestHTTPShapeKeepsNullabilityAndHidesSecrets(t *testing.T) {
	type sample struct {
		embeddedContract
		Items    []string        `json:"items"`
		Optional *bool           `json:"optional,omitempty"`
		Settings json.RawMessage `json:"settings"`
		Secret   string          `json:"-"`
	}
	got := shape(reflect.TypeOf(sample{}))
	for _, field := range []string{"id: number", "items: Array<string> | null", "optional?: boolean | null", "settings: unknown"} {
		if !strings.Contains(got, field) {
			t.Errorf("missing %q in %s", field, got)
		}
	}
	if strings.Contains(got, "Secret") {
		t.Fatal("secret emitted")
	}
}
