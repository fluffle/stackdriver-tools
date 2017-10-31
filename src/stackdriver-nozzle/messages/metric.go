package messages

import (
	"bytes"
	"sort"
	"time"

	"github.com/cloudfoundry/sonde-go/events"
)

type Metric struct {
	Name      string
	Value     float64
	EventTime time.Time
	Unit      string // TODO Should this be "1" if it's empty?
}

// MetricEvent represents the translation of an events.Envelope into a set
// of Metrics
type MetricEvent struct {
	Labels  map[string]string `json:"-"`
	Metrics []*Metric
	Type    events.Envelope_EventType `json:"-"`
}

// Hash returns a string fingerprint that can be used to dedupe MetricEvents.
// A MetricEvent with the same Hash may have different values and timestams
// to other MetricEvents with the same hash.
func (me *MetricEvent) Hash() string {
	var b bytes.Buffer

	// Write all metric names.
	for _, m := range me.Metrics {
		b.WriteString(m.Name)
	}

	// Extract keys to a slice and sort it
	keys := make([]string, len(me.Labels), len(me.Labels))
	for k, _ := range me.Labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	// Write sorted kv pairs.
	for _, k := range keys {
		b.WriteString(k)
		b.WriteString(me.Labels[k])
	}
	return b.String()
}
