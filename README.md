# tdigestbench
[![CircleCI](https://circleci.com/gh/cep21/tdigestbench.svg)](https://circleci.com/gh/cep21/tdigestbench)
[![GoDoc](https://godoc.org/github.com/cep21/tdigestbench?status.svg)](https://godoc.org/github.com/cep21/tdigestbench)
[![codecov](https://codecov.io/gh/cep21/tdigestbench/branch/master/graph/badge.svg)](https://codecov.io/gh/cep21/tdigestbench)

Trying out various tdigest implementations in Go.  Please let me know if you have one you want me to test.
The ones I test are

* [caio/go-tdigest](https://github.com/caio/go-tdigest)
* [segmentio/tdigest](https://github.com/segmentio/tdigest)

# Usage

## Benchmarks
`make bench`

## Correctness
`make test`


# Testing methodology

## Sources

Numeric sources include random numbers, normal distributions, repeating sequences, linearly growing sequences, and
sequences that tend to "spike" 90% of the time.

# Contributing

Tell people how they can contribute.  Start with something simple and create a `CONTRIBUTING.md` file if you really
need it.

Contributions welcome!  Submit a pull request on github and make sure your code passes `make lint test`.  For
large changes, I strongly recommend [creating an issue](https://github.com/cep21/tdigestbench/issues) on GitHub first to
confirm your change will be accepted before writing a lot of code.  GitHub issues are also recommended, at your discretion,
for smaller changes or questions.

# License

This library is licensed under the Apache 2.0 License.