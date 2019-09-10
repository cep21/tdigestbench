package tdigestbench

import (
	"fmt"
	"math"
	"math/rand"
	"sort"
	"testing"
	"time"

	caiot "github.com/caio/go-tdigest"
	influx "github.com/influxdata/tdigest"
	segmentio "github.com/segmentio/tdigest"
)

type allwaysCorrectQuantile struct {
	values []float64
}

func (l *allwaysCorrectQuantile) Add(f float64) {
	l.values = append(l.values, f)
}

func (l *allwaysCorrectQuantile) Quantile(f float64) float64 {
	if len(l.values) == 0 {
		return math.NaN()
	}
	sort.Float64s(l.values)
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

var sizes = []int{1000, 1_000_000}
var quantiles = []float64{0, .1, .5, .9, .99, .999}

type digestRun struct {
	name   string
	digest func() commonTdigest
}

var digests = []digestRun{
	{
		name: "caio",
		digest: func() commonTdigest {
			c, err := caiot.New(caiot.LocalRandomNumberGenerator(0), caiot.Compression(1000))
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

func BenchmarkTdigest_TotalSize(b *testing.B) {
	b.ReportAllocs()
	for _, td := range digests {
		b.Run(fmt.Sprintf("digest=%s", td.name), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				b.ReportAllocs()
				d := td.digest()
				s := sources[2].source()
				for i := 0; i < 100000; i++ {
					d.Add(s.Float64())
				}
			}
		})
	}
}

func BenchmarkTdigest_Add(b *testing.B) {
	b.ReportAllocs()
	for _, source := range sources {
		b.Run(fmt.Sprintf("source=%s", source.name), func(b *testing.B) {
			for _, td := range digests {
				b.Run(fmt.Sprintf("digest=%s", td.name), func(b *testing.B) {
					addBenchmark(b, source.source(), td.digest())
				})
			}
		})
	}
}

func addBenchmark(b *testing.B, source numberSource, tdigest commonTdigest) {
	for i := 0; i < b.N; i++ {
		tdigest.Add(source.Float64())
	}
}

func BenchmarkTdigest_Quantile(b *testing.B) {
	const sourceNum = 0
	b.ReportAllocs()
	for _, s := range digests {
		b.Run(fmt.Sprintf("digest=%s", s.name), func(b *testing.B) {
			source := rand.New(rand.NewSource(sourceNum))
			quantileBenchmark(b, source, s.digest())
		})
	}
}

func quantileBenchmark(b *testing.B, source numberSource, tdigest commonTdigest) {
	for i := 0; i < b.N; i++ {
		tdigest.Quantile(source.Float64())
	}
}

func BenchmarkCorrectness(b *testing.B) {
	for _, size := range sizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			for _, source := range sources {
				b.Run(fmt.Sprintf("source=%s", source.name), func(b *testing.B) {
					for _, td := range digests {
						b.Run(fmt.Sprintf("digest=%s", td.name), func(b *testing.B) {
							correctnessTest(b, size, source.source(), td.digest(), quantiles)
						})
					}
				})
			}
		})
	}
}

func correctnessTest(b *testing.B, size int, source numberSource, tdigest commonTdigest, quants []float64) {
	l := &allwaysCorrectQuantile{}
	for i := 0; i < size; i++ {
		s := source.Float64()
		tdigest.Add(s)
		l.Add(s)
	}
	for _, quant := range quants {
		b.Run(fmt.Sprintf("quant=%f", quant), func(b *testing.B) {
			res := tdigest.Quantile(quant)
			correct := l.Quantile(quant)
			num := math.Abs(res-correct) / ((math.Abs(res) + math.Abs(correct)) / 2)
			if math.IsNaN(num) {
				num = 0
			}
			num = math.Abs(num) * 100
			b.ReportMetric(num, "%difference")
		})
	}
}
