// Package sutl is the dependency-free Go implementation of sUTL 1.0,
// the sUTL Universal Transform Language (“subtle”).
package sutl

// Version is the implementation version and matches the sUTL 1.0.0 contract.
const Version = "1.0.0"

// Value is a MAS value: a map, list, string, number, boolean, or null.
type Value = any

// Library is a named collection of transforms.
type Library map[string]Value

// AsMap reports whether value is a MAS map and returns its entries.
// Objects decoded with DecodeJSON are maps.
func AsMap(value Value) (map[string]Value, bool) {
	return asMap(value)
}

// Evaluate runs a transform against a source value and optional library.
func Evaluate(source, transform Value, library Library) Value {
	return NewRunner().Evaluate(source, transform, library)
}

// Truthy reports sUTL truthiness.
func Truthy(value Value) bool {
	return truthy(value)
}

// CompileLib resolves declaration requirements into a transform library.
// It returns either {"lib": library} or {"fail": failures}.
func CompileLib(declarations []Value, distributions [][]Value, seed Library, test bool) map[string]Value {
	return compileLib(declarations, distributions, seed, test)
}
