package main

import (
	"reflect"
	"testing"

	"github.com/git-pkgs/brief"
)

func TestSchemaForType_CoversAllReportFields(t *testing.T) {
	defs := make(map[string]any)
	schema := schemaForType(reflect.TypeFor[brief.Report](), defs)

	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatal("schema missing properties")
	}

	// Every exported, JSON-tagged field on Report should appear in the schema.
	rt := reflect.TypeFor[brief.Report]()
	for i := range rt.NumField() {
		f := rt.Field(i)
		if !f.IsExported() {
			continue
		}
		tag := f.Tag.Get("json")
		if tag == "-" {
			continue
		}
		name := tag
		if idx := len(name); idx > 0 {
			if comma := indexOf(name, ','); comma >= 0 {
				name = name[:comma]
			}
		}
		if name == "" {
			name = f.Name
		}
		if _, ok := props[name]; !ok {
			t.Errorf("schema missing field %q (struct field %s)", name, f.Name)
		}
	}
}

func TestSchemaForType_GeneratesDefs(t *testing.T) {
	defs := make(map[string]any)
	schemaForType(reflect.TypeFor[brief.Report](), defs)

	// Should have defs for nested struct types like Detection, Command, etc.
	expectedDefs := []string{"detection", "command", "script", "stats"}
	for _, name := range expectedDefs {
		if _, ok := defs[name]; !ok {
			t.Errorf("expected $defs to contain %q", name)
		}
	}
}

func TestSchemaForType_PrimitiveTypes(t *testing.T) {
	defs := make(map[string]any)

	s := schemaForType(reflect.TypeFor[string](), defs)
	if s["type"] != "string" {
		t.Errorf("string type = %v", s)
	}

	i := schemaForType(reflect.TypeFor[int](), defs)
	if i["type"] != "integer" {
		t.Errorf("int type = %v", i)
	}

	f := schemaForType(reflect.TypeFor[float64](), defs)
	if f["type"] != "number" {
		t.Errorf("float64 type = %v", f)
	}

	b := schemaForType(reflect.TypeFor[bool](), defs)
	if b["type"] != "boolean" {
		t.Errorf("bool type = %v", b)
	}
}

func indexOf(s string, c byte) int {
	for i := range len(s) {
		if s[i] == c {
			return i
		}
	}
	return -1
}
