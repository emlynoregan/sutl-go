# Timing

Measured on 23 September 2026 with Go 1.27.0, `windows/amd64`, on a 12th Gen Intel Core i5-12600H. One run each, `go test -benchtime 200ms -count=1 -benchmem`. These are single-machine figures, not a published benchmark suite.

The workloads are in `bench_test.go`:

- **Path.** `^$.name` on one record `{name, city, n}`.
- **Struct.** `{"name": "^$.name", "city": "^$.city"}` on that same record.
- **Reduce.** Sum 3,000 integers with `reduce` and `&+`.
- **Core map.** The core-library `map` (a `reduce` that appends to its accumulator) extracting `name` from 3,000 records.

"Before" is the walker at `189c848`, which built a new builtin table on every `Evaluate`. "Evaluate" and "Compile" are the same binary after `7557cc4`. `Compile` times are steady-state `Program.Run` calls; compilation happens once, outside the loop.

| Workload | Before | `Evaluate` | `Compile` then `Run` |
|---|---:|---:|---:|
| Path | 6.68µs, 93 allocs, 9.3KB | 0.55µs, 9 allocs, 496B | 29ns, 0 allocs |
| Struct | 9.52µs, 121 allocs, 11KB | 1.31µs, 19 allocs, 1.3KB | 242ns, 2 allocs, 336B |
| Reduce, 3,000 numbers | 10.0ms, 198k allocs, 11MB | 5.84ms, 81k allocs, 5.9MB | 0.96ms, 12k allocs, 1.1MB |
| Core map, 3,000 records | 107ms, 318k allocs, 95MB | 75ms, 117k allocs, 87MB | 0.15ms, 2.8k allocs, 73KB |

Path and struct had hundreds of thousands of iterations, so those times are stable. Reduce and the core map had only a handful of iterations in 200ms on the old walker, so treat those two "before" numbers as the right order of magnitude.

`Evaluate` is faster because the builtin table is built once, each compact path is parsed once, and a builtin copies only the scope keys it reads. `^` still merges the whole scope, because that path walks the scope.

`Compile` turns a static transform into a Go function. A plain path such as `^$.name` becomes a map lookup. A dynamic `!` stays on the walker until that transform has been seen, then the compiled form is reused.

The core-library `map` was the outlier. It is a `reduce` whose body copies the whole accumulator on every record, so 3,000 records copy millions of list slots. The compiler recognises that append and builds one result slice. About 52ns per record in this run. If the mapped transform reads the accumulator, that fast path is skipped and the copying `reduce` stays, so the results stay the same.

Repeat the current numbers with:

```powershell
go test -bench "Benchmark(Evaluate|Compile)" -benchmem -benchtime 200ms -count=1 -run "^$"
```

The "before" column needs the walker at `189c848`.
