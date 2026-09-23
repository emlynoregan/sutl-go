package sutl_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/emlynoregan/sutl-go"
)

func testdata(t *testing.T, parts ...string) []byte {
	t.Helper()
	path := filepath.Join(append([]string{"testdata", "contract"}, parts...)...)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func decode(t *testing.T, data []byte) sutl.Value {
	t.Helper()
	value, err := sutl.DecodeJSON(data)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func asMap(t *testing.T, value sutl.Value) map[string]sutl.Value {
	t.Helper()
	m, ok := sutl.AsMap(value)
	if !ok {
		t.Fatalf("expected map, got %T", value)
	}
	return m
}

func asList(t *testing.T, value sutl.Value) []sutl.Value {
	t.Helper()
	list, ok := value.([]sutl.Value)
	if !ok {
		t.Fatalf("expected list, got %T", value)
	}
	return list
}

func bundleLibrary(bundle map[string]sutl.Value) sutl.Library {
	library := sutl.Library{}
	if raw, ok := bundle["library"].([]sutl.Value); ok {
		for _, item := range raw {
			declaration, ok := sutl.AsMap(item)
			if !ok {
				continue
			}
			name, _ := declaration["name"].(string)
			if name == "" {
				continue
			}
			if transform, exists := declaration["transform-t"]; exists {
				library[name] = transform
			}
		}
		if declaration, ok := sutl.AsMap(bundle["declaration"]); ok {
			if requires, ok := declaration["requires"].([]sutl.Value); ok {
				for _, requiredValue := range requires {
					required, _ := requiredValue.(string)
					for _, item := range raw {
						declaration, ok := sutl.AsMap(item)
						if !ok {
							continue
						}
						name, _ := declaration["name"].(string)
						if strings.HasPrefix(name, required) {
							library[required] = declaration["transform-t"]
							break
						}
					}
				}
			}
		}
	}
	return library
}

func runCase(t *testing.T, testCase map[string]sutl.Value) sutl.Value {
	t.Helper()
	library := sutl.Library{}
	if raw, ok := sutl.AsMap(testCase["library"]); ok {
		for key, value := range raw {
			library[key] = value
		}
	}
	mode, _ := testCase["mode"].(string)
	if mode == "" {
		mode = "evaluate"
	}
	switch mode {
	case "evaluate":
		return sutl.Evaluate(testCase["source"], testCase["transform"], library)
	case "compilelib_evaluate":
		compiled := sutl.CompileLib(
			[]sutl.Value{testCase["declaration"]},
			distributions(testCase["distributions"]),
			library,
			truthy(testCase["test"]),
		)
		if fail, exists := compiled["fail"]; exists {
			return map[string]sutl.Value{"compile-fail": fail}
		}
		declaration := asMap(t, testCase["declaration"])
		lib, _ := compiled["lib"].(sutl.Library)
		if lib == nil {
			if converted, ok := compiled["lib"].(map[string]sutl.Value); ok {
				lib = sutl.Library(converted)
			}
		}
		return sutl.Evaluate(testCase["source"], declaration["transform-t"], lib)
	case "declaration_test_t":
		compiled := sutl.CompileLib(
			[]sutl.Value{testCase["declaration"]},
			distributions(testCase["distributions"]),
			library,
			true,
		)
		_, failed := compiled["fail"]
		return !failed
	case "studio_fixture_set":
		glob, _ := testCase["fixture_glob"].(string)
		directory := filepath.Join("testdata", "contract", filepath.Dir(glob))
		entries, err := os.ReadDir(directory)
		if err != nil {
			t.Fatal(err)
		}
		evaluated := 0
		var failures []sutl.Value
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
				continue
			}
			evaluated++
			fixture := asMap(t, decode(t, testdata(t, "fixtures", "studio", entry.Name())))
			actual := sutl.Evaluate(fixture["source"], fixture["transform"], bundleLibrary(fixture))
			if !same(actual, fixture["expected"]) {
				id, _ := fixture["id"].(string)
				failures = append(failures, id)
			}
		}
		return map[string]sutl.Value{"evaluated": evaluated, "failures": failures}
	default:
		t.Fatalf("unknown conformance mode: %s", mode)
		return nil
	}
}

func distributions(value sutl.Value) [][]sutl.Value {
	raw, ok := value.([]sutl.Value)
	if !ok {
		return nil
	}
	out := make([][]sutl.Value, 0, len(raw))
	for _, item := range raw {
		if list, ok := item.([]sutl.Value); ok {
			out = append(out, list)
		}
	}
	return out
}

func truthy(value sutl.Value) bool {
	b, ok := value.(bool)
	return ok && b
}

func same(left, right sutl.Value) bool {
	if isNumber(left) && isNumber(right) {
		return asFloat(left) == asFloat(right)
	}
	if lm, lok := sutl.AsMap(left); lok {
		rm, rok := sutl.AsMap(right)
		if !rok || len(lm) != len(rm) {
			return false
		}
		for key, value := range lm {
			if !same(value, rm[key]) {
				return false
			}
		}
		return true
	}
	switch l := left.(type) {
	case bool:
		r, ok := right.(bool)
		return ok && l == r
	case nil:
		return right == nil
	case string:
		r, ok := right.(string)
		return ok && l == r
	case []sutl.Value:
		r, ok := right.([]sutl.Value)
		if !ok || len(l) != len(r) {
			return false
		}
		for i := range l {
			if !same(l[i], r[i]) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func isNumber(value sutl.Value) bool {
	switch value.(type) {
	case int, int8, int16, int32, int64, float32, float64, json.Number:
		return true
	default:
		return false
	}
}

func asFloat(value sutl.Value) float64 {
	switch n := value.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case json.Number:
		f, _ := n.Float64()
		return f
	default:
		return 0
	}
}

func TestContractAndCorpusChecksum(t *testing.T) {
	contractBytes := testdata(t, "contract.json")
	corpusBytes := testdata(t, "conformance.json")
	contract := asMap(t, decode(t, contractBytes))
	corpus := asMap(t, decode(t, corpusBytes))
	if contract["language"] != "sUTL" || contract["version"] != "1.0.0" {
		t.Fatalf("unexpected contract identity: %#v", contract)
	}
	if contract["conformance_format"] != corpus["format"] {
		t.Fatalf("corpus format %v does not match contract", corpus["format"])
	}
	cases := asList(t, corpus["cases"])
	if len(cases) != 88 {
		t.Fatalf("expected 88 cases, got %d", len(cases))
	}
	sum := sha256.Sum256(corpusBytes)
	if hex.EncodeToString(sum[:]) != contract["corpus_sha256"] {
		t.Fatalf("corpus checksum %s != %s", hex.EncodeToString(sum[:]), contract["corpus_sha256"])
	}
}

func TestConformance(t *testing.T) {
	corpus := asMap(t, decode(t, testdata(t, "conformance.json")))
	for _, raw := range asList(t, corpus["cases"]) {
		testCase := asMap(t, raw)
		id, _ := testCase["id"].(string)
		t.Run(id, func(t *testing.T) {
			actual := runCase(t, testCase)
			if !same(actual, testCase["expected"]) {
				actualJSON, _ := json.Marshal(actual)
				expectedJSON, _ := json.Marshal(testCase["expected"])
				t.Fatalf("expected %s, got %s", expectedJSON, actualJSON)
			}
		})
	}
}

func TestCompiledMatchesEvaluate(t *testing.T) {
	corpus := asMap(t, decode(t, testdata(t, "conformance.json")))
	for _, raw := range asList(t, corpus["cases"]) {
		testCase := asMap(t, raw)
		id, _ := testCase["id"].(string)
		mode, _ := testCase["mode"].(string)
		if mode == "" {
			mode = "evaluate"
		}
		switch mode {
		case "evaluate":
			library := sutl.Library{}
			if rawLib, ok := sutl.AsMap(testCase["library"]); ok {
				for key, value := range rawLib {
					library[key] = value
				}
			}
			want := sutl.Evaluate(testCase["source"], testCase["transform"], library)
			got := sutl.Compile(testCase["transform"], library).Run(testCase["source"])
			if !same(got, want) {
				gotJSON, _ := json.Marshal(got)
				wantJSON, _ := json.Marshal(want)
				t.Fatalf("%s: compiled %s, evaluate %s", id, gotJSON, wantJSON)
			}
		case "studio_fixture_set":
			glob, _ := testCase["fixture_glob"].(string)
			directory := filepath.Join("testdata", "contract", filepath.Dir(glob))
			entries, err := os.ReadDir(directory)
			if err != nil {
				t.Fatal(err)
			}
			for _, entry := range entries {
				if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
					continue
				}
				entry := entry
				t.Run(entry.Name(), func(t *testing.T) {
					fixture := asMap(t, decode(t, testdata(t, "fixtures", "studio", entry.Name())))
					library := bundleLibrary(fixture)
					want := sutl.Evaluate(fixture["source"], fixture["transform"], library)
					got := sutl.Compile(fixture["transform"], library).Run(fixture["source"])
					if !same(got, want) {
						gotJSON, _ := json.Marshal(got)
						wantJSON, _ := json.Marshal(want)
						t.Fatalf("compiled %s, evaluate %s", gotJSON, wantJSON)
					}
				})
			}
		}
	}
}

func TestPublicAPI(t *testing.T) {
	if sutl.Version != "1.0.0" {
		t.Fatalf("version %s", sutl.Version)
	}
	if got := sutl.Evaluate(map[string]sutl.Value{"x": 3}, "^$.x", nil); !same(got, 3) {
		t.Fatalf("evaluate returned %#v", got)
	}
	if !sutl.Truthy([]sutl.Value{0}) {
		t.Fatal("expected nonempty list to be truthy")
	}
	if sutl.NewRunner().Evaluate(nil, []sutl.Value{"&+", 2, 3}, nil) != 5.0 && !same(sutl.Evaluate(nil, []sutl.Value{"&+", 2, 3}, nil), 5) {
		t.Fatalf("runner add failed: %#v", sutl.NewRunner().Evaluate(nil, []sutl.Value{"&+", 2, 3}, nil))
	}
}
