package config

import (
	"encoding/json"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// jsonFieldNames returns the json property names of a struct, following
// embedded structs the way encoding/json promotes them.
func jsonFieldNames(t reflect.Type) []string {
	var names []string
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.Anonymous {
			names = append(names, jsonFieldNames(f.Type)...)
			continue
		}
		tag := strings.Split(f.Tag.Get("json"), ",")[0]
		if tag != "" && tag != "-" {
			names = append(names, tag)
		}
	}
	return names
}

func schemaProperties(t *testing.T, path string, drill ...string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("schema not readable: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("schema %s is not valid JSON: %v", path, err)
	}
	node := doc
	for _, key := range drill {
		child, ok := node[key].(map[string]any)
		if !ok {
			t.Fatalf("schema %s: missing %v", path, drill)
		}
		node = child
	}
	props, ok := node["properties"].(map[string]any)
	if !ok {
		t.Fatalf("schema %s: no properties under %v", path, drill)
	}
	var names []string
	for k := range props {
		names = append(names, k)
	}
	return names
}

func assertSameSet(t *testing.T, what string, structFields, schemaProps []string) {
	t.Helper()
	sort.Strings(structFields)
	sort.Strings(schemaProps)
	if !reflect.DeepEqual(structFields, schemaProps) {
		t.Errorf("%s drifted:\n  Go struct: %v\n  schema:    %v", what, structFields, schemaProps)
	}
}

// The published schemas must match the Go structs exactly, in both
// directions — a new config field without a schema update (or vice versa)
// fails here.
func TestSchemasMatchStructs(t *testing.T) {
	const wtSchema = "../../schema/wt.schema.json"
	const wtrcSchema = "../../schema/wtrc.schema.json"

	assertSameSet(t, "global config (File)",
		jsonFieldNames(reflect.TypeOf(File{})),
		schemaProperties(t, wtSchema))

	assertSameSet(t, "repos entry (RepoConfig)",
		jsonFieldNames(reflect.TypeOf(RepoConfig{})),
		schemaProperties(t, wtSchema, "properties", "repos", "items"))

	assertSameSet(t, "repo-local config (LocalConfig)",
		jsonFieldNames(reflect.TypeOf(LocalConfig{})),
		schemaProperties(t, wtrcSchema))

	assertSameSet(t, "workspace_paths entry (WorkspacePath)",
		jsonFieldNames(reflect.TypeOf(WorkspacePath{})),
		schemaProperties(t, wtSchema, "properties", "workspace_paths", "items"))
}
