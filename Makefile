# https://www.gnu.org/software/make/manual/html_node/Phony-Targets.html
.PHONY: build test test_coverage codecov_coverage format lint bench setup_ci

# Build code with readonly to verify go.mod is up to date in CI
build:
	go build -mod=readonly ./...

# test code with race detector.  Also tests benchmarks (but only for 1ns so they at least run once)
test:
	env "GORACE=halt_on_error=1" go test -v -benchtime 1ns -bench . -race ./...

# Test code with coverage.  Separate from 'test' since covermode=atomic is slow.
test_coverage:
	env "GORACE=halt_on_error=1" go test -v -benchtime 1ns -bench . -covermode=count -coverprofile=coverage.out ./...

# Notice how I directly curl a SHA1 version of codecov-bash
codecov_coverage: test_coverage
	curl -s https://raw.githubusercontent.com/codecov/codecov-bash/1044b7a243e0ea0c05ed43c2acd8b7bb7cef340c/codecov | bash -s -- -f coverage.out  -Z

# Format your code.  Uses both gofmt and goimports
format:
	gofmt -s -w ./..
	find . -iname '*.go' -print0 | xargs -0 goimports -w

# Lint code for static code checking.  Uses golangci-lint
lint:
	golangci-lint run

# Bench runs benchmarks.  The ^$ means it runs no tests, only benchmarks
bench:
	go test -v -run=^$$ -bench=. ./...

# The exact version of CI tools should be specified in your go.mod file and referenced inside your tools.go file
setup_ci:
	go install github.com/cep21/benchdraw
	go install github.com/golangci/golangci-lint/cmd/golangci-lint

# Generate benchmarks and save them to a file for 'draw'
benchresult:
	go test -run=^$$ -bench=. ./... > benchresult.txt

# draw benchmarks as svg files
draw:
	rm -f ./pics/*.svg
	benchdraw --filter="BenchmarkTdigest_Add/size=1000000" --x=digest --y=allocs/op < benchresult.txt > pics/add_memory.svg
	benchdraw --filter="BenchmarkTdigest_Add/size=1000000" --x=digest --y=B/op < benchresult.txt > pics/total_memory.svg
	benchdraw --filter="BenchmarkTdigest_Add/size=1000000/source=exponential" --x=digest --y=B/op < benchresult.txt > pics/total_memory_exponential.svg
	benchdraw --filter="BenchmarkTdigest_Add/size=1000000" --x=source < benchresult.txt > pics/add_timing.svg

	benchdraw --filter="BenchmarkCorrectness/size=1000000/quantile=0.900000" --x=source --y=%difference < benchresult.txt > pics/correct_all.svg
	benchdraw --filter="BenchmarkCorrectness/size=1000000/quantile=0.990000" --x=source --y=%difference < benchresult.txt > pics/correct_all_99.svg
	benchdraw --filter="BenchmarkCorrectness/size=1000000" --x=digest --y=%difference < benchresult.txt > pics/correct_all_all.svg
	benchdraw --filter="BenchmarkCorrectness/size=1000000/digest=segmentio" --v=3 --x=source --y=%difference < benchresult.txt > pics/correct_segment_allq.svg
	benchdraw --filter="BenchmarkCorrectness/size=1000000/digest=influxdata" --v=3 --x=source --y=%difference < benchresult.txt > pics/correct_influx_allq.svg
	benchdraw --filter="BenchmarkCorrectness/size=1000000/digest=caio" --v=3 --x=source --y=%difference < benchresult.txt > pics/correct_caio_allq.svg
	benchdraw --filter="BenchmarkCorrectness/size=1000000/source=exponential" --v=3 --x=digest --y=%difference < benchresult.txt > pics/exponential_source_all.svg
	benchdraw --filter="BenchmarkCorrectness/size=1000000/source=exponential/digest=influxdata" --v=3 --x=quantile --y=%difference < benchresult.txt > pics/exponential_source_influx.svg
	benchdraw --filter="BenchmarkCorrectness/size=1000000/source=exponential/digest=caio" --v=3 --x=quantile --y=%difference < benchresult.txt > pics/exponential_source_caio.svg
