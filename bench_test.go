package sutl_test

import (
	"testing"

	"github.com/emlynoregan/sutl-go"
)

func benchSource() map[string]sutl.Value {
	return map[string]sutl.Value{"name": "Ada", "city": "London", "n": 7}
}

func benchRecords(n int) []sutl.Value {
	records := make([]sutl.Value, n)
	for i := 0; i < n; i++ {
		records[i] = map[string]sutl.Value{"name": "Ada", "city": "London", "n": i}
	}
	return records
}

func benchNumbers(n int) []sutl.Value {
	numbers := make([]sutl.Value, n)
	for i := 0; i < n; i++ {
		numbers[i] = i
	}
	return numbers
}

func coreMapLibrary() sutl.Library {
	return sutl.Library{
		"map": map[string]sutl.Value{
			"&":     "reduce",
			"list":  "^@.list",
			"accum": []sutl.Value{},
			"map-t": "^@.t",
			"t": map[string]sutl.Value{
				"'": []sutl.Value{
					"&&",
					"^@.accum",
					[]sutl.Value{map[string]sutl.Value{"!": "^@.map-t"}},
				},
			},
		},
	}
}

func coreMapTransform() map[string]sutl.Value {
	return map[string]sutl.Value{
		"!":    []sutl.Value{"^*", "map"},
		"list": "^$",
		"t":    map[string]sutl.Value{"'": "^@.item.name"},
	}
}

func reduceSumTransform() map[string]sutl.Value {
	return map[string]sutl.Value{
		"&":     "reduce",
		"list":  "^$",
		"accum": 0,
		"t": map[string]sutl.Value{
			"'": []sutl.Value{"&+", "^@.accum", "^@.item"},
		},
	}
}

func structTransform() map[string]sutl.Value {
	return map[string]sutl.Value{
		"name": "^$.name",
		"city": "^$.city",
	}
}

func TestBenchShapes(t *testing.T) {
	if got := sutl.Evaluate(benchSource(), "^$.name", nil); got != "Ada" {
		t.Fatalf("path %#v", got)
	}
	got := sutl.Evaluate(benchSource(), structTransform(), nil)
	if !same(got, map[string]sutl.Value{"name": "Ada", "city": "London"}) {
		t.Fatalf("struct %#v", got)
	}
	if got := sutl.Evaluate(benchNumbers(4), reduceSumTransform(), nil); !same(got, 6) {
		t.Fatalf("reduce %#v", got)
	}
	got = sutl.Evaluate(benchRecords(2), coreMapTransform(), coreMapLibrary())
	if !same(got, []sutl.Value{"Ada", "Ada"}) {
		t.Fatalf("map %#v", got)
	}
}

func TestCompiledBenchShapes(t *testing.T) {
	if got := sutl.Compile("^$.name", nil).Run(benchSource()); got != "Ada" {
		t.Fatalf("path %#v", got)
	}
	source := map[string]sutl.Value{"name": "Ada", "a": "city", "notfirst": true}
	if got := sutl.Compile("^$.name", nil).Run(source); got != nil {
		t.Fatalf("shadowed path %#v", got)
	}
	if got := sutl.Evaluate(source, "^$.name", nil); got != nil {
		t.Fatalf("shadowed evaluate %#v", got)
	}
	if got := sutl.Compile(structTransform(), nil).Run(benchSource()); !same(got, map[string]sutl.Value{"name": "Ada", "city": "London"}) {
		t.Fatalf("struct %#v", got)
	}
	if got := sutl.Compile(reduceSumTransform(), nil).Run(benchNumbers(4)); !same(got, 6) {
		t.Fatalf("reduce %#v", got)
	}
	if got := sutl.Compile(coreMapTransform(), coreMapLibrary()).Run(benchRecords(2)); !same(got, []sutl.Value{"Ada", "Ada"}) {
		t.Fatalf("map %#v", got)
	}
	reading := map[string]sutl.Value{
		"!":    []sutl.Value{"^*", "map"},
		"list": "^$",
		"t":    map[string]sutl.Value{"'": "^@.accum"},
	}
	numbers := []sutl.Value{1, 2}
	if got, want := sutl.Compile(reading, coreMapLibrary()).Run(numbers), sutl.Evaluate(numbers, reading, coreMapLibrary()); !same(got, want) {
		t.Fatalf("accum-reading map compiled %#v evaluate %#v", got, want)
	}
}

func BenchmarkEvaluatePath(b *testing.B) {
	source := benchSource()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		sutl.Evaluate(source, "^$.name", nil)
	}
}

func BenchmarkEvaluateStruct(b *testing.B) {
	source := benchSource()
	transform := structTransform()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		sutl.Evaluate(source, transform, nil)
	}
}

func BenchmarkEvaluateReduce(b *testing.B) {
	source := benchNumbers(3000)
	transform := reduceSumTransform()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		sutl.Evaluate(source, transform, nil)
	}
}

func BenchmarkEvaluateCoreMap(b *testing.B) {
	source := benchRecords(3000)
	transform := coreMapTransform()
	library := coreMapLibrary()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		sutl.Evaluate(source, transform, library)
	}
}

func BenchmarkCompilePath(b *testing.B) {
	source := benchSource()
	program := sutl.Compile("^$.name", nil)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		program.Run(source)
	}
}

func BenchmarkCompileStruct(b *testing.B) {
	source := benchSource()
	program := sutl.Compile(structTransform(), nil)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		program.Run(source)
	}
}

func BenchmarkCompileReduce(b *testing.B) {
	source := benchNumbers(3000)
	program := sutl.Compile(reduceSumTransform(), nil)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		program.Run(source)
	}
}

func BenchmarkCompileCoreMap(b *testing.B) {
	source := benchRecords(3000)
	program := sutl.Compile(coreMapTransform(), coreMapLibrary())
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		program.Run(source)
	}
}
