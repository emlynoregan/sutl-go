package sutl

import (
	"encoding/json"
	"math"
	"sort"
	"strconv"
	"strings"
)

type builtin func(parent Value, scope map[string]Value, library Library, source, root Value) Value

// Runner evaluates sUTL transforms with an optional transform library.
type Runner struct {
	builtins map[string]builtin
}

// NewRunner constructs a reusable evaluator.
func NewRunner() *Runner {
	r := &Runner{}
	r.builtins = r.makeBuiltins()
	return r
}

// Evaluate runs a transform against a source value and optional library.
func (r *Runner) Evaluate(source, transform Value, library Library) Value {
	if library == nil {
		library = Library{}
	}
	return r.evaluate(source, transform, library, source, transform)
}

func isMap(value Value) bool {
	switch value.(type) {
	case map[string]Value, Library, *object:
		return true
	default:
		return false
	}
}

func isList(value Value) bool {
	_, ok := value.([]Value)
	return ok
}

func isString(value Value) bool {
	_, ok := value.(string)
	return ok
}

func isNumber(value Value) bool {
	switch value.(type) {
	case int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64,
		float32, float64, json.Number:
		return true
	default:
		return false
	}
}

func asFloat(value Value) (float64, bool) {
	switch n := value.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int8:
		return float64(n), true
	case int16:
		return float64(n), true
	case int32:
		return float64(n), true
	case int64:
		return float64(n), true
	case uint:
		return float64(n), true
	case uint8:
		return float64(n), true
	case uint16:
		return float64(n), true
	case uint32:
		return float64(n), true
	case uint64:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	default:
		return 0, false
	}
}

func isWholeNumber(value Value) (int, bool) {
	f, ok := asFloat(value)
	if !ok || math.Trunc(f) != f {
		return 0, false
	}
	return int(f), true
}

func truthy(value Value) bool {
	switch x := value.(type) {
	case nil:
		return false
	case bool:
		return x
	case string:
		return x != ""
	case []Value:
		return len(x) > 0
	case map[string]Value:
		return len(x) > 0
	case Library:
		return len(x) > 0
	case *object:
		return len(x.m) > 0
	default:
		if f, ok := asFloat(value); ok {
			return f != 0
		}
		return value != nil
	}
}

func asMap(value Value) (map[string]Value, bool) {
	switch m := value.(type) {
	case map[string]Value:
		return m, true
	case Library:
		return map[string]Value(m), true
	case *object:
		return m.m, true
	default:
		return nil, false
	}
}

func asList(value Value) ([]Value, bool) {
	list, ok := value.([]Value)
	return list, ok
}

func asString(value Value) (string, bool) {
	s, ok := value.(string)
	return s, ok
}

func copyMap(value Value) map[string]Value {
	if m, ok := asMap(value); ok {
		out := make(map[string]Value, len(m))
		for key, item := range m {
			out[key] = item
		}
		return out
	}
	return map[string]Value{}
}

func deepCopy(value Value) Value {
	switch x := value.(type) {
	case *object:
		out := &object{keys: append([]string{}, x.keys...), m: make(map[string]Value, len(x.m))}
		for _, key := range x.keys {
			out.m[key] = deepCopy(x.m[key])
		}
		return out
	case Library:
		return deepCopy(map[string]Value(x))
	case map[string]Value:
		out := make(map[string]Value, len(x))
		for key, item := range x {
			out[key] = deepCopy(item)
		}
		return out
	case []Value:
		out := make([]Value, len(x))
		for i, item := range x {
			out[i] = deepCopy(item)
		}
		return out
	default:
		return value
	}
}

func pathStep(values Value, selector Value) []Value {
	list, ok := asList(values)
	if !ok {
		return []Value{}
	}
	if selector == nil {
		return list
	}
	if s, ok := asString(selector); ok && s == "" {
		return list
	}

	result := []Value{}
	for _, value := range list {
		if selector == "**" {
			result = append(result, value)
			stack := []Value{value}
			for len(stack) > 0 {
				current := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				if isMap(current) {
					for _, child := range objectValues(current) {
						result = append(result, child)
						stack = append(stack, child)
					}
				} else if items, ok := asList(current); ok {
					result = append(result, items...)
					stack = append(stack, items...)
				}
			}
			continue
		}
		if selector == "*" {
			if isMap(value) {
				result = append(result, objectValues(value)...)
			} else if items, ok := asList(value); ok {
				result = append(result, items...)
			}
			continue
		}
		if m, ok := asMap(value); ok {
			if key, ok := asString(selector); ok {
				if item, exists := m[key]; exists {
					result = append(result, item)
				}
			}
			continue
		}
		if items, ok := asList(value); ok {
			if index, ok := isWholeNumber(selector); ok && index >= 0 && index < len(items) {
				result = append(result, items[index])
			}
		}
	}
	return result
}

func (r *Runner) evaluate(scope, transform Value, library Library, source, root Value) Value {
	if m, ok := asMap(transform); ok {
		if _, exists := m["!"]; exists {
			return r.evaluateEval(scope, m, library, source, root)
		}
		if _, exists := m["!!"]; exists {
			return r.evaluateEval2(scope, m, library, source, root)
		}
		if _, exists := m["&"]; exists {
			return r.evaluateBuiltin(scope, m, library, source, root)
		}
		if quoted, exists := m["'"]; exists {
			return r.quote(scope, quoted, library, source, root)
		}
		if literal, exists := m[":"]; exists {
			return literal
		}
		return r.evaluateMap(scope, m, library, source, root)
	}
	if r.isCompactBuiltin(transform) {
		return r.evaluateCompact(scope, transform, library, source, root)
	}
	if list, ok := asList(transform); ok {
		values := list
		flatten := len(list) > 0 && list[0] == "&&"
		if flatten {
			values = list[1:]
		}
		result := make([]Value, 0, len(values))
		for _, item := range values {
			result = append(result, r.evaluate(scope, item, library, source, root))
		}
		if flatten {
			return flattenList(result)
		}
		return result
	}
	return transform
}

func (r *Runner) quote(scope, transform Value, library Library, source, root Value) Value {
	if m, ok := asMap(transform); ok {
		if escaped, exists := m["''"]; exists {
			return r.evaluate(scope, escaped, library, source, root)
		}
		out := make(map[string]Value, len(m))
		for key, value := range m {
			out[key] = r.quote(scope, value, library, source, root)
		}
		return out
	}
	if list, ok := asList(transform); ok {
		out := make([]Value, len(list))
		for i, value := range list {
			out[i] = r.quote(scope, value, library, source, root)
		}
		return out
	}
	return transform
}

func (r *Runner) evaluateMap(scope Value, transform map[string]Value, library Library, source, root Value) map[string]Value {
	out := make(map[string]Value)
	for key, value := range transform {
		if key == "!" || key == "&" {
			continue
		}
		out[key] = r.evaluate(scope, value, library, source, root)
	}
	return out
}

func (r *Runner) evaluateEval(scope Value, transform map[string]Value, library Library, source, root Value) Value {
	nextTransform := r.evaluate(scope, transform["!"], library, source, root)
	nextScope := copyMap(scope)
	for key, value := range r.evaluateMap(scope, transform, library, source, root) {
		nextScope[key] = value
	}
	nextLibrary := library
	if inner, ok := asMap(transform["*"]); ok {
		nextLibrary = Library(r.evaluateMap(scope, inner, library, source, root))
	}
	return r.evaluate(nextScope, nextTransform, nextLibrary, source, root)
}

func (r *Runner) evaluateEval2(scope Value, transform map[string]Value, library Library, source, root Value) Value {
	nextTransform := r.evaluate(scope, transform["!!"], library, source, root)
	nextScope := scope
	if _, exists := transform["s"]; exists {
		delta := r.evaluate(scope, transform["s"], library, source, root)
		if dm, ok := asMap(delta); ok {
			merged := copyMap(scope)
			for key, value := range dm {
				merged[key] = value
			}
			nextScope = merged
		} else {
			nextScope = delta
		}
	}
	nextLibrary := library
	if inner, ok := asMap(transform["*"]); ok {
		nextLibrary = Library(r.evaluateMap(scope, inner, library, source, root))
	}
	return r.evaluate(nextScope, nextTransform, nextLibrary, source, root)
}

func (r *Runner) isCompactBuiltin(transform Value) bool {
	var parts []Value
	if s, ok := asString(transform); ok {
		for _, part := range strings.Split(s, ".") {
			parts = append(parts, part)
		}
	} else if list, ok := asList(transform); ok {
		parts = list
	} else {
		return false
	}
	if len(parts) == 0 {
		return false
	}
	operation, ok := asString(parts[0])
	if !ok || operation == "" {
		return false
	}
	prefix := operation[:1]
	return (prefix == "&" || prefix == "^") && r.builtins[operation[1:]] != nil
}

func (r *Runner) evaluateCompact(scope, transform Value, library Library, source, root Value) Value {
	var parts []Value
	if s, ok := asString(transform); ok {
		for _, part := range strings.Split(s, ".") {
			if n, err := strconv.Atoi(part); err == nil {
				parts = append(parts, n)
			} else {
				parts = append(parts, part)
			}
		}
	} else {
		parts = append(parts, transform.([]Value)...)
	}
	operation := parts[0].(string)
	return r.evaluateBuiltin(scope, map[string]Value{
		"&":    operation[1:],
		"args": parts[1:],
		"head": operation[:1] == "^",
	}, library, source, root)
}

func (r *Runner) evaluateBuiltin(scope Value, transform map[string]Value, library Library, source, root Value) Value {
	if arguments, ok := asList(transform["args"]); ok {
		var result Value
		name := transform["&"]
		if len(arguments) == 0 {
			result = r.evaluateBuiltin(scope, map[string]Value{"&": name}, library, source, root)
		} else if len(arguments) == 1 {
			result = r.evaluateBuiltin(scope, map[string]Value{
				"&": name,
				"b": r.evaluate(scope, arguments[0], library, source, root),
			}, library, source, root)
		} else {
			result = r.evaluate(scope, arguments[0], library, source, root)
			for index, item := range arguments[1:] {
				result = r.evaluateBuiltin(scope, map[string]Value{
					"&":        name,
					"a":        result,
					"b":        r.evaluate(scope, item, library, source, root),
					"notfirst": index > 0,
				}, library, source, root)
			}
		}
		if truthy(transform["head"]) {
			if list, ok := asList(result); ok && len(list) > 0 {
				return list[0]
			}
			return nil
		}
		return result
	}

	name, _ := asString(transform["&"])
	fn := r.builtins[name]
	libraryName := name
	if fn != nil {
		libraryName = "_override_" + name
	}
	if _, exists := library[libraryName]; exists {
		call := copyMap(transform)
		call["!"] = []Value{"^*", libraryName}
		delete(call, "&")
		return r.evaluateEval(scope, call, library, source, root)
	}
	if fn == nil {
		return nil
	}

	nextScope := copyMap(scope)
	for key, value := range r.evaluateMap(scope, transform, library, source, root) {
		nextScope[key] = value
	}
	nextLibrary := library
	if inner, ok := asMap(transform["*"]); ok {
		nextLibrary = Library(r.evaluateMap(scope, inner, library, source, root))
	}
	return fn(scope, nextScope, nextLibrary, source, root)
}

func flattenList(values []Value) []Value {
	result := []Value{}
	for _, value := range values {
		if list, ok := asList(value); ok {
			result = append(result, list...)
		} else {
			result = append(result, value)
		}
	}
	return result
}

func get(scope map[string]Value, key string, fallback Value) Value {
	value, exists := scope[key]
	if !exists || value == nil {
		return fallback
	}
	return value
}

func equal(left, right Value) bool {
	if isNumber(left) && isNumber(right) {
		lf, lok := asFloat(left)
		rf, rok := asFloat(right)
		return lok && rok && lf == rf
	}
	if isString(left) && isString(right) {
		return left == right
	}
	return same(left, right)
}

func add(left, right Value) (Value, bool) {
	if isNumber(left) && isNumber(right) {
		lf, _ := asFloat(left)
		rf, _ := asFloat(right)
		return lf + rf, true
	}
	if isString(left) && isString(right) {
		return left.(string) + right.(string), true
	}
	return nil, false
}

func numericOp(left, right Value, op func(float64, float64) Value) Value {
	if !isNumber(left) || !isNumber(right) {
		return nil
	}
	lf, lok := asFloat(left)
	rf, rok := asFloat(right)
	if !lok || !rok {
		return nil
	}
	return op(lf, rf)
}

func compare(left, right Value, op func(float64, float64) bool) bool {
	if !isNumber(left) || !isNumber(right) {
		return false
	}
	lf, lok := asFloat(left)
	rf, rok := asFloat(right)
	return lok && rok && op(lf, rf)
}

func stringValue(value Value) string {
	switch x := value.(type) {
	case string:
		return x
	case bool:
		if x {
			return "true"
		}
		return "false"
	case nil:
		return "null"
	case map[string]Value, Library, *object:
		return "map"
	case []Value:
		return "list"
	default:
		if f, ok := asFloat(value); ok {
			if math.Trunc(f) == f {
				return strconv.FormatInt(int64(f), 10)
			}
			return strconv.FormatFloat(f, 'f', -1, 64)
		}
		return "unknown"
	}
}

func numberValue(value Value) Value {
	if isNumber(value) {
		if f, ok := asFloat(value); ok {
			return f
		}
	}
	if s, ok := asString(value); ok {
		if n, err := strconv.Atoi(s); err == nil {
			return n
		}
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			return f
		}
		return 0
	}
	if b, ok := value.(bool); ok {
		if b {
			return 1
		}
		return 0
	}
	return 0
}

func typeValue(value Value) string {
	switch {
	case isMap(value):
		return "map"
	case isList(value):
		return "list"
	case isString(value):
		return "string"
	case isNumber(value):
		return "number"
	default:
		switch value.(type) {
		case bool:
			return "boolean"
		case nil:
			return "null"
		default:
			return "unknown"
		}
	}
}

func processPath(start Value, scope map[string]Value) []Value {
	left := scope["a"]
	right := scope["b"]
	if truthy(scope["notfirst"]) {
		return pathStep(left, right)
	}
	return pathStep(pathStep([]Value{start}, left), right)
}

func removeKeys(mapping, keys Value) Value {
	if mapping == nil {
		return nil
	}
	result := deepCopy(mapping)
	m, ok := asMap(result)
	if !ok {
		return nil
	}
	if list, ok := asList(keys); ok {
		for _, key := range list {
			if s, ok := asString(key); ok {
				delete(m, s)
				if obj, ok := result.(*object); ok {
					obj.keys = withoutKey(obj.keys, s)
				}
			}
		}
	}
	return result
}

func withoutKey(keys []string, drop string) []string {
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		if key != drop {
			out = append(out, key)
		}
	}
	return out
}

func makeMap(value Value) Value {
	list, ok := asList(value)
	if !ok {
		return nil
	}
	out := map[string]Value{}
	for _, item := range list {
		pair, ok := asList(item)
		if !ok || len(pair) < 2 {
			continue
		}
		key, ok := asString(pair[0])
		if !ok {
			continue
		}
		out[key] = pair[1]
	}
	return out
}

func splitValue(value, separator, maximum Value) Value {
	if !truthy(value) {
		return nil
	}
	if truthy(maximum) && !isNumber(maximum) {
		return nil
	}
	sep := ","
	if truthy(separator) {
		sep = stringValue(separator)
	}
	text := stringValue(value)
	if truthy(maximum) {
		n, ok := isWholeNumber(maximum)
		if !ok {
			return nil
		}
		return splitN(text, sep, n)
	}
	parts := strings.Split(text, sep)
	out := make([]Value, len(parts))
	for i, part := range parts {
		out[i] = part
	}
	return out
}

func splitN(text, sep string, maximum int) []Value {
	// Python str.split(sep, max) yields at most max+1 parts.
	parts := strings.SplitN(text, sep, maximum+1)
	out := make([]Value, len(parts))
	for i, part := range parts {
		out[i] = part
	}
	return out
}

func position(value, substring Value) Value {
	if !truthy(value) || !truthy(substring) {
		return nil
	}
	return strings.Index(stringValue(value), stringValue(substring))
}

func zipLists(value Value) Value {
	rows, ok := asList(value)
	if !ok {
		return []Value{}
	}
	if len(rows) == 0 {
		return []Value{}
	}
	length := -1
	converted := make([][]Value, 0, len(rows))
	for _, row := range rows {
		list, ok := asList(row)
		if !ok {
			return nil
		}
		if length == -1 || len(list) < length {
			length = len(list)
		}
		converted = append(converted, list)
	}
	if length < 0 {
		length = 0
	}
	out := make([]Value, 0, length)
	for i := 0; i < length; i++ {
		tuple := make([]Value, len(converted))
		for j, row := range converted {
			tuple[j] = deepCopy(row[i])
		}
		out = append(out, tuple)
	}
	return out
}

func sortedKeys(mapping map[string]Value) []string {
	keys := make([]string, 0, len(mapping))
	for key := range mapping {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func (r *Runner) makeBuiltins() map[string]builtin {
	binary := func(left, right func(map[string]Value) Value, op func(Value, Value) Value) builtin {
		return func(parent Value, scope map[string]Value, library Library, source, root Value) Value {
			return op(left(scope), right(scope))
		}
	}
	unary := func(value func(map[string]Value) Value, op func(Value) Value) builtin {
		return func(parent Value, scope map[string]Value, library Library, source, root Value) Value {
			return op(value(scope))
		}
	}

	builtins := map[string]builtin{
		"+": binary(
			func(s map[string]Value) Value { return get(s, "a", 0) },
			func(s map[string]Value) Value { return get(s, "b", 0) },
			func(a, b Value) Value {
				result, ok := add(a, b)
				if !ok {
					return nil
				}
				return result
			},
		),
		"-": binary(
			func(s map[string]Value) Value { return get(s, "a", 0) },
			func(s map[string]Value) Value { return get(s, "b", 0) },
			func(a, b Value) Value {
				return numericOp(a, b, func(x, y float64) Value { return x - y })
			},
		),
		"x": binary(
			func(s map[string]Value) Value { return get(s, "a", 1) },
			func(s map[string]Value) Value { return get(s, "b", 1) },
			func(a, b Value) Value {
				return numericOp(a, b, func(x, y float64) Value { return x * y })
			},
		),
		"/": binary(
			func(s map[string]Value) Value { return get(s, "a", 1) },
			func(s map[string]Value) Value { return get(s, "b", 1) },
			func(a, b Value) Value {
				return numericOp(a, b, func(x, y float64) Value {
					if y == 0 {
						return nil
					}
					return x / y
				})
			},
		),
		"=": binary(
			func(s map[string]Value) Value { return get(s, "a", nil) },
			func(s map[string]Value) Value { return get(s, "b", nil) },
			func(a, b Value) Value { return equal(a, b) },
		),
		"!=": binary(
			func(s map[string]Value) Value { return get(s, "a", nil) },
			func(s map[string]Value) Value { return get(s, "b", nil) },
			func(a, b Value) Value { return !equal(a, b) },
		),
		">": binary(
			func(s map[string]Value) Value { return s["a"] },
			func(s map[string]Value) Value { return s["b"] },
			func(a, b Value) Value { return compare(a, b, func(x, y float64) bool { return x > y }) },
		),
		"<": binary(
			func(s map[string]Value) Value { return s["a"] },
			func(s map[string]Value) Value { return s["b"] },
			func(a, b Value) Value { return compare(a, b, func(x, y float64) bool { return x < y }) },
		),
		">=": binary(
			func(s map[string]Value) Value { return s["a"] },
			func(s map[string]Value) Value { return s["b"] },
			func(a, b Value) Value { return compare(a, b, func(x, y float64) bool { return x >= y }) },
		),
		"<=": binary(
			func(s map[string]Value) Value { return s["a"] },
			func(s map[string]Value) Value { return s["b"] },
			func(a, b Value) Value { return compare(a, b, func(x, y float64) bool { return x <= y }) },
		),
		"&&": binary(
			func(s map[string]Value) Value { return get(s, "a", false) },
			func(s map[string]Value) Value { return get(s, "b", false) },
			func(a, b Value) Value { return truthy(a) && truthy(b) },
		),
		"||": binary(
			func(s map[string]Value) Value { return get(s, "a", false) },
			func(s map[string]Value) Value { return get(s, "b", false) },
			func(a, b Value) Value { return truthy(a) || truthy(b) },
		),
		"!": unary(
			func(s map[string]Value) Value { return get(s, "b", false) },
			func(value Value) Value { return !truthy(value) },
		),
	}

	builtins["if"] = func(parent Value, scope map[string]Value, library Library, source, root Value) Value {
		branch := "false"
		if truthy(scope["cond"]) {
			branch = "true"
		}
		value, exists := scope[branch]
		if !exists {
			return nil
		}
		return r.evaluate(parent, value, library, source, root)
	}
	builtins["reduce"] = func(parent Value, scope map[string]Value, library Library, source, root Value) Value {
		accumulator := scope["accum"]
		if values, ok := asList(scope["list"]); ok {
			for index, item := range values {
				itemScope := copyMap(parent)
				for key, value := range scope {
					itemScope[key] = value
				}
				itemScope["item"] = item
				itemScope["accum"] = accumulator
				itemScope["ix"] = index
				accumulator = r.evaluate(itemScope, scope["t"], library, source, root)
			}
		}
		return accumulator
	}
	builtins["$"] = func(parent Value, scope map[string]Value, library Library, source, root Value) Value {
		return processPath(source, scope)
	}
	builtins["@"] = func(parent Value, scope map[string]Value, library Library, source, root Value) Value {
		return processPath(parent, scope)
	}
	builtins["^"] = func(parent Value, scope map[string]Value, library Library, source, root Value) Value {
		return processPath(scope, scope)
	}
	builtins["*"] = func(parent Value, scope map[string]Value, library Library, source, root Value) Value {
		return processPath(library, scope)
	}
	builtins["~"] = func(parent Value, scope map[string]Value, library Library, source, root Value) Value {
		return processPath(root, scope)
	}
	builtins["%"] = func(parent Value, scope map[string]Value, library Library, source, root Value) Value {
		left := scope["a"]
		right := scope["b"]
		if truthy(scope["notfirst"]) {
			return pathStep(left, right)
		}
		if left == nil {
			return pathStep([]Value{right}, nil)
		}
		return pathStep([]Value{left}, right)
	}
	builtins["zip"] = func(parent Value, scope map[string]Value, library Library, source, root Value) Value {
		return zipLists(scope["list"])
	}
	builtins["removekeys"] = func(parent Value, scope map[string]Value, library Library, source, root Value) Value {
		return removeKeys(scope["map"], scope["keys"])
	}
	builtins["len"] = func(parent Value, scope map[string]Value, library Library, source, root Value) Value {
		if list, ok := asList(scope["list"]); ok {
			return len(list)
		}
		return 0
	}
	builtins["keys"] = func(parent Value, scope map[string]Value, library Library, source, root Value) Value {
		m, ok := asMap(scope["map"])
		if !ok {
			return nil
		}
		keys := sortedKeys(m)
		out := make([]Value, len(keys))
		for i, key := range keys {
			out[i] = key
		}
		return out
	}
	builtins["values"] = func(parent Value, scope map[string]Value, library Library, source, root Value) Value {
		m, ok := asMap(scope["map"])
		if !ok {
			return nil
		}
		keys := sortedKeys(m)
		out := make([]Value, len(keys))
		for i, key := range keys {
			out[i] = m[key]
		}
		return out
	}
	builtins["type"] = func(parent Value, scope map[string]Value, library Library, source, root Value) Value {
		return typeValue(scope["value"])
	}
	builtins["makemap"] = func(parent Value, scope map[string]Value, library Library, source, root Value) Value {
		return makeMap(scope["value"])
	}
	builtins["quicksort"] = func(parent Value, scope map[string]Value, library Library, source, root Value) Value {
		list, ok := asList(scope["list"])
		if !ok {
			return scope["list"]
		}
		out := append([]Value{}, list...)
		sort.SliceStable(out, func(i, j int) bool {
			if isNumber(out[i]) && isNumber(out[j]) {
				lf, _ := asFloat(out[i])
				rf, _ := asFloat(out[j])
				return lf < rf
			}
			if isString(out[i]) && isString(out[j]) {
				return out[i].(string) < out[j].(string)
			}
			return false
		})
		return out
	}
	builtins["head"] = func(parent Value, scope map[string]Value, library Library, source, root Value) Value {
		if list, ok := asList(scope["b"]); ok && len(list) > 0 {
			return list[0]
		}
		return nil
	}
	builtins["tail"] = func(parent Value, scope map[string]Value, library Library, source, root Value) Value {
		if list, ok := asList(scope["b"]); ok {
			if len(list) <= 1 {
				return []Value{}
			}
			return append([]Value{}, list[1:]...)
		}
		return nil
	}
	builtins["split"] = func(parent Value, scope map[string]Value, library Library, source, root Value) Value {
		return splitValue(scope["value"], scope["sep"], scope["max"])
	}
	builtins["trim"] = func(parent Value, scope map[string]Value, library Library, source, root Value) Value {
		if !truthy(scope["value"]) {
			return nil
		}
		return strings.TrimSpace(stringValue(scope["value"]))
	}
	builtins["pos"] = func(parent Value, scope map[string]Value, library Library, source, root Value) Value {
		return position(scope["value"], scope["sub"])
	}
	builtins["string"] = func(parent Value, scope map[string]Value, library Library, source, root Value) Value {
		return stringValue(scope["value"])
	}
	builtins["number"] = func(parent Value, scope map[string]Value, library Library, source, root Value) Value {
		return numberValue(scope["value"])
	}
	builtins["boolean"] = func(parent Value, scope map[string]Value, library Library, source, root Value) Value {
		return truthy(scope["value"])
	}
	builtins["lower"] = func(parent Value, scope map[string]Value, library Library, source, root Value) Value {
		return strings.ToLower(stringValue(scope["value"]))
	}
	builtins["upper"] = func(parent Value, scope map[string]Value, library Library, source, root Value) Value {
		return strings.ToUpper(stringValue(scope["value"]))
	}

	names := make([]string, 0, len(builtins))
	for name := range builtins {
		names = append(names, name)
	}
	for _, name := range names {
		builtins["has"+name] = func(parent Value, scope map[string]Value, library Library, source, root Value) Value {
			return true
		}
	}
	return builtins
}

func compileLib(declarations []Value, distributions [][]Value, seed Library, test bool) map[string]Value {
	runner := NewRunner()
	result := Library{}
	for key, value := range seed {
		result[key] = value
	}
	var failures []Value

	var addRequirements func(items []Value)
	addRequirements = func(items []Value) {
		for _, raw := range items {
			declaration, ok := asMap(raw)
			if !ok {
				continue
			}
			declaredName, _ := asString(declaration["name"])
			requires, _ := asList(declaration["requires"])
			for _, requiredValue := range requires {
				required, ok := asString(requiredValue)
				if !ok {
					continue
				}
				if _, exists := result[required]; exists {
					continue
				}
				if strings.HasPrefix(declaredName, required) {
					result[required] = declaration["transform-t"]
					continue
				}
				var candidates []map[string]Value
				for _, distribution := range distributions {
					for _, item := range distribution {
						candidate, ok := asMap(item)
						if !ok {
							continue
						}
						name, _ := asString(candidate["name"])
						if strings.HasPrefix(name, required) {
							candidates = append(candidates, candidate)
						}
					}
				}
				if len(candidates) == 0 {
					failures = append(failures, "missing requirement: "+required)
					continue
				}
				var candidateFailures []Value
				for _, candidate := range candidates {
					before := copyMap(result)
					addRequirements([]Value{candidate})
					if test {
						if _, exists := candidate["test-t"]; exists {
							candidateLibrary := Library{}
							if reqs, ok := asList(candidate["requires"]); ok {
								for _, req := range reqs {
									if name, ok := asString(req); ok {
										if value, exists := result[name]; exists {
											candidateLibrary[name] = value
										}
									}
								}
							}
							failure := runner.Evaluate(candidate["transform-t"], candidate["test-t"], candidateLibrary)
							if truthy(failure) {
								candidateFailures = append(candidateFailures, failure)
								result = Library{}
								for key, value := range before {
									result[key] = value
								}
								continue
							}
						}
					}
					result[required] = candidate["transform-t"]
					candidateFailures = nil
					break
				}
				failures = append(failures, candidateFailures...)
			}
		}
	}

	addRequirements(declarations)
	if test {
		for _, raw := range declarations {
			declaration, ok := asMap(raw)
			if !ok {
				continue
			}
			if _, exists := declaration["test-t"]; !exists {
				continue
			}
			declarationLibrary := Library{}
			if reqs, ok := asList(declaration["requires"]); ok {
				for _, req := range reqs {
					if name, ok := asString(req); ok {
						if value, exists := result[name]; exists {
							declarationLibrary[name] = value
						}
					}
				}
			}
			failure := runner.Evaluate(declaration["transform-t"], declaration["test-t"], declarationLibrary)
			if truthy(failure) {
				failures = append(failures, failure)
			}
		}
	}
	if len(failures) > 0 {
		return map[string]Value{"fail": failures}
	}
	return map[string]Value{"lib": result}
}

func same(left, right Value) bool {
	if isNumber(left) && isNumber(right) {
		lf, lok := asFloat(left)
		rf, rok := asFloat(right)
		return lok && rok && lf == rf
	}
	if lm, lok := asMap(left); lok {
		rm, rok := asMap(right)
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
	if ll, lok := asList(left); lok {
		rl, rok := asList(right)
		if !rok || len(ll) != len(rl) {
			return false
		}
		for i := range ll {
			if !same(ll[i], rl[i]) {
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
	default:
		return left == right
	}
}
