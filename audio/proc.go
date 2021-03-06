/*
Copyright 2013 Google Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package audio

import (
	"math"
	"math/rand"

	"github.com/nf/sigourney/fast"
)

func sampleToHz(s Sample) float64 {
	return 440 * fast.Exp2(float64(s)*10)
}

func NewSquare() *Square {
	o := &Square{}
	o.inputs("pitch", &o.pitch, "syn", &o.syn)
	return o
}

type Square struct {
	sink
	pitch Processor // 0.1/oct, 0 == 440Hz
	syn   trigger

	pos float64
}

func (o *Square) Process(s []Sample) {
	o.pitch.Process(s)
	t := o.syn.Process()
	p := o.pos
	hz, lastS := sampleToHz(s[0]), s[0]
	for i := range s {
		if o.syn.isTrigger(t[i]) {
			p = 0
		}
		if s[i] != lastS {
			hz = sampleToHz(s[i])
		}
		p += hz
		if p > waveHz {
			p -= waveHz
		}
		if p > waveHz/2 {
			s[i] = -1
		} else {
			s[i] = 1
		}
	}
	o.pos = p
}

func NewSin() *Sin {
	o := &Sin{}
	o.inputs("pitch", &o.pitch, "syn", &o.syn)
	return o
}

type Sin struct {
	sink
	pitch Processor // 0.1/oct, 0 == 440Hz
	syn   trigger

	pos float64
}

func (o *Sin) Process(s []Sample) {
	o.pitch.Process(s)
	t := o.syn.Process()
	p := o.pos
	hz, lastS := sampleToHz(s[0]), s[0]
	for i := range s {
		if o.syn.isTrigger(t[i]) {
			p = 0
		}
		if s[i] != lastS {
			hz = sampleToHz(s[i])
		}
		s[i] = Sample(fast.Sin(p * 2 * math.Pi))
		p += hz / waveHz
		if p > 100 {
			p -= 100
		}
	}
	o.pos = p
}

func NewMul() *Mul {
	a := &Mul{}
	a.inputs("a", &a.a, "b", &a.b)
	return a
}

type Mul struct {
	sink
	a Processor
	b source
}

func (a *Mul) Process(s []Sample) {
	a.a.Process(s)
	m := a.b.Process()
	for i := range s {
		s[i] *= m[i]
	}
}

func NewSum() *Sum {
	s := &Sum{}
	s.inputs("a", &s.a, "b", &s.b)
	return s
}

type Sum struct {
	sink
	a Processor
	b source
}

func (s *Sum) Process(a []Sample) {
	s.a.Process(a)
	b := s.b.Process()
	for i := range a {
		a[i] += b[i]
	}
}

func NewMulSum() *MulSum {
	m := &MulSum{}
	m.inputs("a", &m.a, "b", &m.b, "x", &m.x)
	return m
}

type MulSum struct {
	sink
	a    Processor
	b, x source
}

func (m *MulSum) Process(s []Sample) {
	m.a.Process(s)
	b, x := m.b.Process(), m.x.Process()
	for i := range s {
		s[i] *= x[i]
		s[i] += b[i]
	}
}

func NewEnv() *Env {
	e := &Env{}
	e.inputs("gate", &e.gate, "trig", &e.trig, "att", &e.att, "dec", &e.dec)
	return e
}

type Env struct {
	sink
	gate     Processor
	trig     trigger
	att, dec source

	v      Sample
	up     bool
	wasLow bool
}

func (e *Env) Process(s []Sample) {
	e.gate.Process(s)
	att, dec, t := e.att.Process(), e.dec.Process(), e.trig.Process()
	v := e.v
	for i := range s {
		if e.trig.isTrigger(t[i]) {
			e.up = true
		}
		if !e.up && v > s[i] {
			if d := dec[i]; d > 0 {
				v -= 1 / (d * waveHz * 10)
				if v < s[i] {
					v = s[i]
				}
			} else {
				v = s[i]
			}
		}
		if e.up || v < s[i] {
			if a := att[i]; a > 0 {
				v += 1 / (a * waveHz * 10)
				if e.up {
					if v > 1 {
						v = 1
						e.up = false
					}
				} else if v > s[i] {
					v = s[i]
				}
			} else if e.up {
				v = 1
				e.up = false
			} else if v < s[i] {
				v = s[i]
			}
		}
		s[i] = v
	}
	e.v = v
}

func NewClip() *Clip {
	c := &Clip{}
	c.inputs("in", &c.in)
	return c
}

type Clip struct {
	sink
	in Processor
}

func (c *Clip) Process(s []Sample) {
	c.in.Process(s)
	for i, v := range s {
		if v > 1 {
			s[i] = 1
		} else if v < -1 {
			s[i] = -1
		}
	}
}

type Value Sample

func (v Value) Process(s []Sample) {
	for i := range s {
		s[i] = Sample(v)
	}
}

func NewRand() *Rand {
	r := &Rand{}
	r.inputs("min", &r.min, "max", &r.max, "trig", &r.trig)
	return r
}

type Rand struct {
	sink
	min  Processor
	max  source
	trig trigger

	last Sample
}

func (r *Rand) Process(s []Sample) {
	r.min.Process(s)
	max, t := r.max.Process(), r.trig.Process()
	v := r.last
	for i := range s {
		if r.trig.isTrigger(t[i]) {
			v = s[i] + Sample(rand.Float64())*(max[i]-s[i])
		}
		s[i] = v
	}
	r.last = v
}

func NewDelay() *Delay {
	d := &Delay{buf: make([]Sample, waveHz)}
	d.inputs("in", &d.in, "len", &d.len)
	return d
}

type Delay struct {
	sink
	in  Processor
	len source

	p   int
	buf []Sample
}

func (d *Delay) Process(s []Sample) {
	d.in.Process(s)
	l := d.len.Process()
	p := d.p
	for i := range s {
		max := int(l[i] * waveHz)
		if max < FrameLength {
			continue
		}
		if max > waveHz {
			max = waveHz
		}
		if p >= max {
			p = 0
		}
		s[i], d.buf[p] = d.buf[p], s[i]
		p++

	}
	d.p = p
}

func NewQuant() *Quant {
	q := &Quant{}
	q.inputs("in", &q.in)
	return q
}

type Quant struct {
	sink
	in Processor
}

func (q *Quant) Process(s []Sample) {
	q.in.Process(s)
	for i := range s {
		s[i] = Sample(int(s[i]*120)) / 120
	}
}

func NewSkip() *Skip {
	s := &Skip{}
	s.inputs("num", &s.num, "trig", &s.trig)
	return s
}

type Skip struct {
	sink
	num  Processor
	trig trigger

	n int
}

func (s *Skip) Process(b []Sample) {
	s.num.Process(b)
	t := s.trig.Process()
	for i := range b {
		if s.trig.isTrigger(t[i]) {
			m := int(b[i] * 10)
			if m <= 0 || s.n%m == 0 {
				b[i], s.n = 1, 1
				continue
			}
			s.n++
		}
		b[i] = 0
	}
}

func NewStep() *Step {
	s := &Step{}
	s.inputs("trig", &s.trig, "rst", &s.rst, "v", s.in[:])
	return s
}

type Step struct {
	sink
	trig, rst trigger
	in        [nStep]source

	n int
}

const nStep = 4

func (s *Step) Process(b []Sample) {
	t, r := s.trig.Process(), s.rst.Process()
	in := make([][]Sample, 8)
	for i := range s.in {
		in[i] = s.in[i].Process()
	}
	for i := range b {
		if s.trig.isTrigger(t[i]) {
			s.n = (s.n + 1) % nStep
		}
		if s.rst.isTrigger(r[i]) {
			s.n = 0
		}
		b[i] = in[s.n][i]
	}
}

func NewNoise() *Noise {
	return &Noise{}
}

type Noise struct {
}

func (p *Noise) Process(s []Sample) {
	for i := range s {
		s[i] = Sample(rand.Float64()*2 - 1)
	}
}

func NewFilter() *Filter {
	f := &Filter{buf: make([]Sample, maxFilterBufferLength)}
	f.inputs("in", &f.in, "freq", &f.freq)
	return f
}

const maxFilterBufferLength = 1024

type Filter struct {
	sink
	in   Processor
	freq source

	buf  []Sample // Circular buffer.
	bufp int      // Pointer into circular buffer.
	avg  Sample   // Rolling average.
}

// TODO(adg): write a better circular buffer implementation that can
// grow and shrink as necessary, while maintaining its circularity.

func (f *Filter) Process(s []Sample) {
	f.in.Process(s)
	freq := f.freq.Process()

	avg, buf, bufp := f.avg, f.buf, f.bufp
	n, lastFreq := filterBufferLength(freq[0]), freq[0]
	for i := range s {
		// Recompute n if freq has changed.
		if freq[i] != lastFreq {
			n, lastFreq = filterBufferLength(freq[i]), freq[i]
		}
		// Add current sample and subtract last sample.
		cur := s[i] / Sample(n)
		avg += cur - buf[bufp]
		// Replace last sample in buffer with current sample.
		buf[bufp] = cur
		// Bump buffer pointer.
		bufp++
		bufp %= int(n)
		// Use rolling average as output.
		s[i] = avg
	}
	f.avg, f.buf, f.bufp = avg, buf, bufp
}

func filterBufferLength(freq Sample) Sample {
	s := Sample(waveHz / sampleToHz(freq))
	if s >= maxFilterBufferLength {
		return maxFilterBufferLength
	}
	if s < 0 {
		return 0
	}
	return s

}
