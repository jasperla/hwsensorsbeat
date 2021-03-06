package helper

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/elastic/beats/libbeat/common"
)

// TestMetricSeterState tests if a metricset persists its state during multiple Fetch requests
func TestMetricSeterState(t *testing.T) {
	module := &Module{}

	metricSet, err := NewMetricSet("mockmetricset", NewMockMetricSeter, module)
	assert.NoError(t, err)

	event, _ := metricSet.MetricSeter.Fetch(metricSet, "")
	assert.Equal(t, 1, event["counter"])

	event, _ = metricSet.MetricSeter.Fetch(metricSet, "")
	assert.Equal(t, 2, event["counter"])
}

// TestMetricSetTwoInstances makes sure that in case of two different MetricSet instance, MetricSeter don't share state
func TestMetricSetTwoInstances(t *testing.T) {
	module := &Module{}

	metricSet1, err1 := NewMetricSet("mockmetricset1", NewMockMetricSeter, module)
	metricSet2, err2 := NewMetricSet("mockmetricset2", NewMockMetricSeter, module)
	assert.NoError(t, err1)
	assert.NoError(t, err2)

	event, _ := metricSet1.MetricSeter.Fetch(metricSet1, "")
	assert.Equal(t, 1, event["counter"])

	event, _ = metricSet2.MetricSeter.Fetch(metricSet2, "")
	assert.Equal(t, 1, event["counter"])
}

/*** Mock tests objects ***/

// New creates new instance of MetricSeter
func NewMockMetricSeter() MetricSeter {
	return &MockMetricSeter{
		counter: 0,
	}
}

type MockMetricSeter struct {
	counter int
}

func (m *MockMetricSeter) Setup(ms *MetricSet) error {
	return nil
}

func (m *MockMetricSeter) Fetch(ms *MetricSet, host string) (event common.MapStr, err error) {
	m.counter += 1

	event = common.MapStr{
		"counter": m.counter,
	}

	return event, nil
}
