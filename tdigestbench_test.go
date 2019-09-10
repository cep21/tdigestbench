package tdigestbench

import (
	"fmt"
	"math"
	"math/rand"
	"os"
	"sort"
	"testing"
	"time"

	caiot "github.com/caio/go-tdigest"
	influx "github.com/influxdata/tdigest"
	segmentio "github.com/segmentio/tdigest"
)

type allwaysCorrectQuantile struct {
	values []float64
	sorted bool
}

func (l *allwaysCorrectQuantile) Add(f float64) {
	l.values = append(l.values, f)
	l.sorted = false
}

func (l *allwaysCorrectQuantile) Quantile(f float64) float64 {
	if len(l.values) == 0 {
		return math.NaN()
	}
	if !l.sorted {
		sort.Float64s(l.values)
		l.sorted = true
	}
	quantileLocation := float64(len(l.values)) * f
	if quantileLocation <= 0 {
		return l.values[0]
	}
	if quantileLocation >= float64(len(l.values)-1) {
		return l.values[len(l.values)-1]
	}
	beforeIndex := int(math.Floor(quantileLocation))
	afterIndex := beforeIndex + 1
	delta := l.values[afterIndex] - l.values[beforeIndex]
	if delta < 0 {
		panic("invalid")
	}
	if delta == 0 {
		return l.values[beforeIndex]
	}
	return l.values[beforeIndex] + delta*(quantileLocation-float64(beforeIndex))
}

var _ commonTdigest = &allwaysCorrectQuantile{}

type commonTdigest interface {
	Add(f float64)
	Quantile(f float64) float64
}

type numberSource interface {
	Float64() float64
}

type linearSource struct {
	at int64
}

func (l *linearSource) Float64() float64 {
	l.at++
	return float64(l.at)
}

// Repeats a sequence forever
type repeatingNumberSource struct {
	at  int
	seq []float64
}

func (l *repeatingNumberSource) Float64() float64 {
	ret := l.seq[l.at]
	l.at++
	if l.at == len(l.seq) {
		l.at = 0
	}
	return ret
}

var _ numberSource = &repeatingNumberSource{}
var _ numberSource = &linearSource{}

var _ numberSource = &rand.Rand{}

type caioTdigest struct {
	c *caiot.TDigest
}

func (c *caioTdigest) Quantile(f float64) float64 {
	return c.c.Quantile(f)
}

func (c *caioTdigest) Add(f float64) {
	if err := c.c.Add(f); err != nil {
		panic(err)
	}
}

type segmentTdigest struct {
	t *segmentio.TDigest
}

func (s *segmentTdigest) Quantile(f float64) float64 {
	return s.t.Quantile(f)
}

func (s *segmentTdigest) Add(f float64) {
	s.t.Add(f, 1)
}

type influxTdigest struct {
	t *influx.TDigest
}

func (s *influxTdigest) Quantile(f float64) float64 {
	return s.t.Quantile(f)
}

func (s *influxTdigest) Add(f float64) {
	s.t.Add(f, 1)
}

type exponentialDistribution struct {
	r *rand.Rand
}

func (h *exponentialDistribution) Float64() float64 {
	return h.r.ExpFloat64()
}

type tailSpikeDistribution struct {
	ratio float64
	r     *rand.Rand
}

func (h *tailSpikeDistribution) Float64() float64 {
	if h.r.Float64() < h.ratio {
		// Return some value that tends to take less than 200 ms
		return h.r.Float64() * (time.Millisecond * 200).Seconds()
		// lower side of hump
	}
	// a spike of values between 1.5 - 2 seconds
	return h.r.Float64()*.5 + 1.5
}

var _ numberSource = &tailSpikeDistribution{}
var _ numberSource = &exponentialDistribution{}
var _ commonTdigest = &caioTdigest{}
var _ commonTdigest = &segmentTdigest{}

type sourceRun struct {
	name   string
	source func() numberSource
}

var sources = []sourceRun{
	{
		name: "linear",
		source: func() numberSource {
			return &linearSource{}
		},
	},
	{
		name: "rand",
		source: func() numberSource {
			return rand.New(rand.NewSource(0))
		},
	},
	{
		name: "alternating",
		source: func() numberSource {
			return &repeatingNumberSource{
				seq: []float64{-1, -1, 1},
			}
		},
	},
	{
		name: "exponential",
		source: func() numberSource {
			return &exponentialDistribution{r: rand.New(rand.NewSource(0))}
		},
	},
	{
		name: "tailspike",
		source: func() numberSource {
			return &tailSpikeDistribution{r: rand.New(rand.NewSource(0)), ratio: .9}
		},
	},
}

func init() {
	if os.Getenv("CIRCLECI") != "" {
		// Otherwise our CI environment just takes too long :(
		sizes = []int{1000}
	}
}

var sizes = []int{1000, 1_000_000}
var quantiles = []float64{.1, .5, .9, .99, .999}

type digestRun struct {
	name   string
	digest func() commonTdigest
}

var digests = []digestRun{
	{
		name: "caio",
		digest: func() commonTdigest {
			// Seed note: This seed is an arbitrary large prime.  I just want to make sure the seed caiot uses
			// is not the seed I use for random number generation (which is zero there)
			const seed = 180811
			c, err := caiot.New(caiot.LocalRandomNumberGenerator(seed), caiot.Compression(1000))
			if err != nil {
				panic(err)
			}
			return &caioTdigest{c: c}
		},
	},
	{
		name: "segmentio",
		digest: func() commonTdigest {
			td := segmentio.NewWithCompression(1000)
			return &segmentTdigest{t: td}
		},
	},
	{
		name: "influxdata",
		digest: func() commonTdigest {
			td := influx.New()
			return &influxTdigest{t: td}
		},
	},
}

func numberArrayFromSource(s numberSource, size int) []float64 {
	ret := make([]float64, size)
	for i := 0; i < size; i++ {
		ret[i] = s.Float64()
	}
	return ret
}

func addNumbersToQuantile(ts commonTdigest, nums []float64) {
	for _, n := range nums {
		ts.Add(n)
	}
}

// Benchmark creating a digest of size b.N, then calling a single set of quantiles on it (we call quantiles just to make sure the digest is used)
func BenchmarkTdigest_Add(b *testing.B) {
	for _, source := range sources {
		b.Run(fmt.Sprintf("source=%s", source.name), func(b *testing.B) {
			for _, td := range digests {
				b.Run(fmt.Sprintf("digest=%s", td.name), func(b *testing.B) {
					nums := numberArrayFromSource(source.source(), b.N)
					b.ReportAllocs()
					b.ResetTimer()
					digestImpl := td.digest()
					addNumbersToQuantile(digestImpl, nums)
					for _, q := range quantiles {
						digestImpl.Quantile(q)
					}
				})
			}
		})
	}
}

// Benchmark Total is the benchmark of a single item with a static size, tested b.N times.
func BenchmarkTdigest_TotalSize(b *testing.B) {
	for _, size := range sizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			for _, source := range sources {
				b.Run(fmt.Sprintf("source=%s", source.name), func(b *testing.B) {
					nums := numberArrayFromSource(source.source(), size)
					for _, td := range digests {
						b.Run(fmt.Sprintf("digest=%s", td.name), func(b *testing.B) {
							b.ReportAllocs()
							b.ResetTimer()
							for i := 0; i < b.N; i++ {
								digestImpl := td.digest()
								addNumbersToQuantile(digestImpl, nums)
								for _, q := range quantiles {
									digestImpl.Quantile(q)
								}
							}
						})
					}
				})
			}
		})
	}
}

// Benchmark calculating quantile b.N times for a static size
func BenchmarkTdigest_Quantile(b *testing.B) {
	for _, size := range sizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			for _, source := range sources {
				b.Run(fmt.Sprintf("source=%s", source.name), func(b *testing.B) {
					nums := numberArrayFromSource(source.source(), size)
					for _, td := range digests {
						b.Run(fmt.Sprintf("digest=%s", td.name), func(b *testing.B) {
							digestImpl := td.digest()
							addNumbersToQuantile(digestImpl, nums)
							b.ReportAllocs()
							b.ResetTimer()
							for i := 0; i < b.N; i++ {
								for _, q := range quantiles {
									digestImpl.Quantile(q)
								}
							}
						})
					}
				})
			}
		})
	}
}

func relativeDifferencePercentile(res, correct float64) float64 {
	num := math.Abs(res-correct) / ((math.Abs(res) + math.Abs(correct)) / 2)
	if math.IsNaN(num) {
		return 0
	}
	return num * 100
}

// Benchmark the correctness of a quantile implementation after adding a static size of numbers.  Calculate correctness
// across multiple quantiles.
func BenchmarkCorrectness(b *testing.B) {
	for _, size := range sizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			for _, source := range sources {
				b.Run(fmt.Sprintf("source=%s", source.name), func(b *testing.B) {
					nums := numberArrayFromSource(source.source(), size)
					perfect := &allwaysCorrectQuantile{}
					addNumbersToQuantile(perfect, nums)
					for _, td := range digests {
						b.Run(fmt.Sprintf("digest=%s", td.name), func(b *testing.B) {
							digestImpl := td.digest()
							addNumbersToQuantile(digestImpl, nums)
							for _, quant := range quantiles {
								b.Run(fmt.Sprintf("quantile=%f", quant), func(b *testing.B) {
									b.ReportMetric(0, "ns/op")
									b.ReportMetric(relativeDifferencePercentile(digestImpl.Quantile(quant), perfect.Quantile(quant)), "%difference")
								})
							}
						})
					}
				})
			}
		})
	}
}
