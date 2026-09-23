package sutl

import (
	"fmt"
	"sync"
)

// Program is a transform compiled to a Go function.
// Compile once, then Run the same transform against many sources.
type Program struct {
	root   Value
	runner *Runner
	fn     compiled
	mu     sync.Mutex
	cache  map[string]compiled
}

type compiled func(scope, source Value) Value

type field struct {
	key string
	run compiled
}

// Compile prepares a transform so it can be applied repeatedly.
// Dynamic eval (! ) stays on the walker until that transform has been seen,
// then the compiled form is reused. A library is captured by reference.
func Compile(transform Value, library Library) *Program {
	if library == nil {
		library = Library{}
	}
	program := &Program{
		root:   transform,
		runner: defaultRunner,
		cache:  map[string]compiled{},
	}
	program.fn = program.compile(transform, library)
	return program
}

// Run applies the compiled transform to source.
func (p *Program) Run(source Value) Value {
	return p.fn(source, source)
}

func (p *Program) compile(transform Value, library Library) compiled {
	if m, ok := asMap(transform); ok {
		if _, exists := m["!"]; exists {
			return p.compileEval(m, library)
		}
		if _, exists := m["!!"]; exists {
			return p.interpret(transform, library)
		}
		if _, exists := m["&"]; exists {
			return p.compileBuiltin(m, library)
		}
		if quoted, exists := m["'"]; exists {
			return p.compileQuote(quoted, library)
		}
		if literal, exists := m[":"]; exists {
			return constCompiled(literal)
		}
		return p.compileMap(m, library)
	}
	if parsed, ok := parseCompact(transform); ok {
		return p.compileCompact(transform, parsed, library)
	}
	if list, ok := asList(transform); ok {
		return p.compileList(list, library)
	}
	return constCompiled(transform)
}

func constCompiled(value Value) compiled {
	return func(scope, source Value) Value {
		return value
	}
}

func (p *Program) interpret(transform Value, library Library) compiled {
	runner := p.runner
	root := p.root
	return func(scope, source Value) Value {
		return runner.evaluate(scope, transform, library, source, root)
	}
}

func (p *Program) compileQuote(quoted Value, library Library) compiled {
	if hasEscape(quoted) {
		runner := p.runner
		root := p.root
		return func(scope, source Value) Value {
			return runner.quote(scope, quoted, library, source, root)
		}
	}
	// Copy once. Later runs reuse this value, so a dynamic eval of it can be cached.
	return constCompiled(p.runner.quote(nil, quoted, library, nil, p.root))
}

func hasEscape(value Value) bool {
	if m, ok := asMap(value); ok {
		if _, exists := m["''"]; exists {
			return true
		}
		for _, child := range m {
			if hasEscape(child) {
				return true
			}
		}
		return false
	}
	if list, ok := asList(value); ok {
		for _, child := range list {
			if hasEscape(child) {
				return true
			}
		}
	}
	return false
}

func (p *Program) compileList(list []Value, library Library) compiled {
	flatten := len(list) > 0 && list[0] == "&&"
	items := list
	if flatten {
		items = list[1:]
	}
	fns := make([]compiled, len(items))
	for i, item := range items {
		fns[i] = p.compile(item, library)
	}
	return func(scope, source Value) Value {
		result := make([]Value, 0, len(fns))
		for _, fn := range fns {
			result = append(result, fn(scope, source))
		}
		if flatten {
			return flattenList(result)
		}
		return result
	}
}

func (p *Program) compileMap(m map[string]Value, library Library) compiled {
	fields := p.compileFields(m, library)
	return func(scope, source Value) Value {
		out := make(map[string]Value, len(fields))
		for _, field := range fields {
			out[field.key] = field.run(scope, source)
		}
		return out
	}
}

func (p *Program) compileFields(m map[string]Value, library Library) []field {
	fields := make([]field, 0, len(m))
	for key, value := range m {
		if key == "!" || key == "&" {
			continue
		}
		fields = append(fields, field{key: key, run: p.compile(value, library)})
	}
	return fields
}

func (p *Program) compileEval(m map[string]Value, library Library) compiled {
	bang := p.compile(m["!"], library)
	fields := p.compileFields(m, library)
	var star compiled
	if inner, ok := asMap(m["*"]); ok {
		star = p.compileMap(inner, library)
	}
	return func(scope, source Value) Value {
		nextTransform := bang(scope, source)
		nextScope := copyMap(scope)
		for _, field := range fields {
			nextScope[field.key] = field.run(scope, source)
		}
		nextLibrary := library
		if star != nil {
			if made, ok := star(scope, source).(map[string]Value); ok {
				nextLibrary = Library(made)
			}
		}
		return p.cached(nextTransform, nextLibrary)(nextScope, source)
	}
}

func (p *Program) cached(transform Value, library Library) compiled {
	key := cacheKey(transform, library)
	p.mu.Lock()
	if fn, ok := p.cache[key]; ok {
		p.mu.Unlock()
		return fn
	}
	p.mu.Unlock()
	fn := p.compile(transform, library)
	p.mu.Lock()
	if existing, ok := p.cache[key]; ok {
		p.mu.Unlock()
		return existing
	}
	p.cache[key] = fn
	p.mu.Unlock()
	return fn
}

func cacheKey(transform Value, library Library) string {
	lib := fmt.Sprintf("%p", map[string]Value(library))
	switch value := transform.(type) {
	case string:
		return "s\x00" + value + "\x00" + lib
	case map[string]Value:
		return fmt.Sprintf("m:%p:%s", value, lib)
	case []Value:
		return fmt.Sprintf("l:%p:%s", value, lib)
	case *object:
		return fmt.Sprintf("o:%p:%s", value, lib)
	case Library:
		return fmt.Sprintf("L:%p:%s", map[string]Value(value), lib)
	default:
		return fmt.Sprintf("v:%T:%#v:%s", transform, transform, lib)
	}
}

func (p *Program) compileBuiltin(m map[string]Value, library Library) compiled {
	if arguments, ok := asList(m["args"]); ok {
		name, _ := asString(m["&"])
		if name == "" || p.overridden(name, library) || p.runner.builtins[name] == nil {
			return p.interpret(m, library)
		}
		args := make([]compiled, len(arguments))
		for i, arg := range arguments {
			args[i] = p.compile(arg, library)
		}
		head := truthy(m["head"])
		if fn := p.compileStaticPath(name, head, arguments, library); fn != nil {
			return fn
		}
		if fn := p.compileFastOp(name, head, args); fn != nil {
			return fn
		}
		return p.compileCall(name, head, args, library)
	}
	name, ok := asString(m["&"])
	if !ok || p.overridden(name, library) || p.runner.builtins[name] == nil {
		return p.interpret(m, library)
	}
	switch name {
	case "reduce":
		return p.compileReduce(m, library)
	case "if":
		return p.compileIf(m, library)
	default:
		return p.compileDirect(name, m, library)
	}
}

func (p *Program) overridden(name string, library Library) bool {
	if len(library) == 0 {
		return false
	}
	libraryName := name
	if p.runner.builtins[name] != nil {
		libraryName = "_override_" + name
	}
	_, exists := library[libraryName]
	return exists
}

func (p *Program) compileCompact(transform Value, parsed parsedCompact, library Library) compiled {
	if p.overridden(parsed.name, library) {
		return p.interpret(transform, library)
	}
	if fn := p.compileStaticPath(parsed.name, parsed.head, parsed.args, library); fn != nil {
		return fn
	}
	args := make([]compiled, len(parsed.args))
	for i, arg := range parsed.args {
		args[i] = p.compile(arg, library)
	}
	if fn := p.compileFastOp(parsed.name, parsed.head, args); fn != nil {
		return fn
	}
	return p.compileCall(parsed.name, parsed.head, args, library)
}

func (p *Program) compileStaticPath(name string, head bool, args []Value, library Library) compiled {
	if !head || !isWalkPath(name) || !plainSegments(args) {
		return nil
	}
	segs := args
	root := p.root
	fallback := p.callFold(name, head, constArgs(segs), library)
	return func(scope, source Value) Value {
		if scopeShadowsPath(scope, len(segs)) {
			return fallback(scope, source)
		}
		current := pathRoot(name, scope, source, library, root)
		for _, seg := range segs {
			next, ok := plainStep(current, seg.(string))
			if !ok {
				return nil
			}
			current = next
		}
		return current
	}
}

func constArgs(values []Value) []compiled {
	fns := make([]compiled, len(values))
	for i, value := range values {
		fns[i] = constCompiled(value)
	}
	return fns
}

func isWalkPath(name string) bool {
	switch name {
	case "$", "@", "*", "~":
		return true
	default:
		return false
	}
}

func plainSegments(args []Value) bool {
	if len(args) == 0 {
		return false
	}
	for _, arg := range args {
		s, ok := arg.(string)
		if !ok || s == "" || s == "*" || s == "**" {
			return false
		}
	}
	return true
}

func scopeShadowsPath(scope Value, nargs int) bool {
	if nargs >= 2 {
		return false
	}
	m, ok := asMap(scope)
	if !ok {
		return false
	}
	if value, exists := m["a"]; exists && value != nil {
		return true
	}
	if nargs == 0 {
		if value, exists := m["b"]; exists && value != nil {
			return true
		}
	}
	value, exists := m["notfirst"]
	return exists && truthy(value)
}

func pathRoot(name string, scope, source Value, library Library, root Value) Value {
	switch name {
	case "$":
		return source
	case "@":
		return scope
	case "*":
		return library
	case "~":
		return root
	default:
		return nil
	}
}

func plainStep(current Value, key string) (Value, bool) {
	m, ok := asMap(current)
	if !ok {
		return nil, false
	}
	item, exists := m[key]
	if !exists {
		return nil, false
	}
	return item, true
}

func (p *Program) compileFastOp(name string, head bool, args []compiled) compiled {
	if head || len(args) != 2 || !isFastOp(name) {
		return nil
	}
	left, right := args[0], args[1]
	return func(scope, source Value) Value {
		return applyBinary(name, left(scope, source), right(scope, source))
	}
}

func isFastOp(name string) bool {
	switch name {
	case "+", "-", "x", "/", "=", "!=", ">", "<", ">=", "<=", "&&", "||":
		return true
	default:
		return false
	}
}

func applyBinary(name string, left, right Value) Value {
	switch name {
	case "+":
		result, ok := add(present(left, 0), present(right, 0))
		if !ok {
			return nil
		}
		return result
	case "-":
		return numericOp(present(left, 0), present(right, 0), func(x, y float64) Value { return x - y })
	case "x":
		return numericOp(present(left, 1), present(right, 1), func(x, y float64) Value { return x * y })
	case "/":
		return numericOp(present(left, 1), present(right, 1), func(x, y float64) Value {
			if y == 0 {
				return nil
			}
			return x / y
		})
	case "=":
		return equal(left, right)
	case "!=":
		return !equal(left, right)
	case ">":
		return compare(left, right, func(x, y float64) bool { return x > y })
	case "<":
		return compare(left, right, func(x, y float64) bool { return x < y })
	case ">=":
		return compare(left, right, func(x, y float64) bool { return x >= y })
	case "<=":
		return compare(left, right, func(x, y float64) bool { return x <= y })
	case "&&":
		return truthy(present(left, false)) && truthy(present(right, false))
	case "||":
		return truthy(present(left, false)) || truthy(present(right, false))
	default:
		return nil
	}
}

func present(value, fallback Value) Value {
	if value == nil {
		return fallback
	}
	return value
}

func (p *Program) compileCall(name string, head bool, args []compiled, library Library) compiled {
	return p.callFold(name, head, args, library)
}

func (p *Program) callFold(name string, head bool, args []compiled, library Library) compiled {
	fn := p.runner.builtins[name]
	root := p.root
	return func(scope, source Value) Value {
		var result Value
		switch len(args) {
		case 0:
			result = fn(scope, scopeForBuiltin(name, scope, map[string]Value{}), library, source, root)
		case 1:
			result = fn(scope, scopeForBuiltin(name, scope, map[string]Value{
				"b": args[0](scope, source),
			}), library, source, root)
		default:
			result = args[0](scope, source)
			for i := 1; i < len(args); i++ {
				result = fn(scope, scopeForBuiltin(name, scope, map[string]Value{
					"a":        result,
					"b":        args[i](scope, source),
					"notfirst": i > 1,
				}), library, source, root)
			}
		}
		if head {
			return headValue(result)
		}
		return result
	}
}

func headValue(result Value) Value {
	if list, ok := asList(result); ok && len(list) > 0 {
		return list[0]
	}
	return nil
}

func (p *Program) compileDirect(name string, m map[string]Value, library Library) compiled {
	fn := p.runner.builtins[name]
	fields := p.compileFields(m, library)
	var star compiled
	if inner, ok := asMap(m["*"]); ok {
		star = p.compileMap(inner, library)
	}
	root := p.root
	return func(scope, source Value) Value {
		evaluated := make(map[string]Value, len(fields))
		for _, field := range fields {
			evaluated[field.key] = field.run(scope, source)
		}
		nextLibrary := library
		if star != nil {
			if made, ok := star(scope, source).(map[string]Value); ok {
				nextLibrary = Library(made)
			}
		}
		return fn(scope, scopeForBuiltin(name, scope, evaluated), nextLibrary, source, root)
	}
}

func (p *Program) compileIf(m map[string]Value, library Library) compiled {
	fields := p.compileFields(m, library)
	var star compiled
	if inner, ok := asMap(m["*"]); ok {
		star = p.compileMap(inner, library)
	}
	return func(scope, source Value) Value {
		evaluated := make(map[string]Value, len(fields))
		for _, field := range fields {
			evaluated[field.key] = field.run(scope, source)
		}
		next := scopeForBuiltin("if", scope, evaluated)
		nextLibrary := library
		if star != nil {
			if made, ok := star(scope, source).(map[string]Value); ok {
				nextLibrary = Library(made)
			}
		}
		branch := "false"
		if truthy(next["cond"]) {
			branch = "true"
		}
		value, exists := next[branch]
		if !exists {
			return nil
		}
		return p.cached(value, nextLibrary)(scope, source)
	}
}

func (p *Program) compileReduce(m map[string]Value, library Library) compiled {
	fields := p.compileFields(m, library)
	var star compiled
	if inner, ok := asMap(m["*"]); ok {
		star = p.compileMap(inner, library)
	}
	return func(scope, source Value) Value {
		evaluated := make(map[string]Value, len(fields))
		for _, field := range fields {
			evaluated[field.key] = field.run(scope, source)
		}
		evaluated = scopeForBuiltin("reduce", scope, evaluated)
		nextLibrary := library
		if star != nil {
			if made, ok := star(scope, source).(map[string]Value); ok {
				nextLibrary = Library(made)
			}
		}
		accumulator := evaluated["accum"]
		items, ok := asList(evaluated["list"])
		if !ok {
			return accumulator
		}
		if initial, isList := asList(accumulator); isList {
			if element, matched := matchAppendBody(evaluated["t"]); matched && !elementReadsAccum(element, evaluated) {
				return p.appendReduce(element, initial, items, scope, source, evaluated, nextLibrary)
			}
		}
		body := p.cached(evaluated["t"], nextLibrary)
		for index, item := range items {
			itemScope := copyMap(scope)
			for key, value := range evaluated {
				itemScope[key] = value
			}
			itemScope["item"] = item
			itemScope["accum"] = accumulator
			itemScope["ix"] = index
			accumulator = body(itemScope, source)
		}
		return accumulator
	}
}

// matchAppendBody recognises the core-library map step: append one value to accum.
// The shape is ["&&", "^@.accum", [element]].
func matchAppendBody(body Value) (Value, bool) {
	list, ok := asList(body)
	if !ok || len(list) != 3 || list[0] != "&&" || list[1] != "^@.accum" {
		return nil, false
	}
	wrapped, ok := asList(list[2])
	if !ok || len(wrapped) != 1 {
		return nil, false
	}
	return wrapped[0], true
}

func elementReadsAccum(element Value, evaluated map[string]Value) bool {
	if target, ok := staticBang(element, evaluated); ok {
		return readsAccum(target)
	}
	return readsAccum(element)
}

func staticBang(element Value, evaluated map[string]Value) (Value, bool) {
	m, ok := asMap(element)
	if !ok || len(m) != 1 {
		return nil, false
	}
	bang, exists := m["!"]
	if !exists {
		return nil, false
	}
	path, ok := asString(bang)
	if !ok {
		return nil, false
	}
	switch path {
	case "^@.map-t":
		return evaluated["map-t"], true
	case "^@.t":
		return evaluated["t"], true
	default:
		return nil, false
	}
}

func readsAccum(value Value) bool {
	if text, ok := asString(value); ok {
		parsed, ok := parseCompact(text)
		return ok && compactReadsAccum(parsed)
	}
	if list, ok := asList(value); ok {
		if parsed, ok := parseCompact(list); ok {
			return compactReadsAccum(parsed)
		}
		for _, item := range list {
			if readsAccum(item) {
				return true
			}
		}
		return false
	}
	m, ok := asMap(value)
	if !ok {
		return false
	}
	if _, exists := m["!"]; exists {
		return true
	}
	if _, exists := m["!!"]; exists {
		return true
	}
	if name, _ := asString(m["&"]); name == "@" || name == "^" {
		return true
	}
	for _, child := range m {
		if readsAccum(child) {
			return true
		}
	}
	return false
}

func compactReadsAccum(parsed parsedCompact) bool {
	if parsed.name != "@" && parsed.name != "^" {
		return false
	}
	if len(parsed.args) == 0 {
		return true
	}
	first, ok := parsed.args[0].(string)
	if !ok {
		return true
	}
	return first == "accum" || first == "*" || first == "**"
}

func (p *Program) appendReduce(element Value, initial, items []Value, scope, source Value, evaluated map[string]Value, library Library) Value {
	buf := make([]Value, len(initial), len(initial)+len(items))
	copy(buf, initial)
	itemScope := copyMap(scope)
	for key, value := range evaluated {
		itemScope[key] = value
	}
	if target, ok := staticBang(element, evaluated); ok {
		fn := p.cached(target, library)
		for index, item := range items {
			itemScope["item"] = item
			itemScope["ix"] = index
			buf = append(buf, fn(itemScope, source))
		}
		return buf
	}
	fn := p.compile(element, library)
	for index, item := range items {
		itemScope["item"] = item
		itemScope["ix"] = index
		buf = append(buf, fn(itemScope, source))
	}
	return buf
}
