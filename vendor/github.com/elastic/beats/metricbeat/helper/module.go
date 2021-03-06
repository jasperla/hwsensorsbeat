package helper

import (
	"sort"
	"sync"
	"time"

	"fmt"

	"github.com/elastic/beats/libbeat/beat"
	"github.com/elastic/beats/libbeat/common"
	"github.com/elastic/beats/libbeat/logp"

	"github.com/urso/ucfg"
)

// Module specifics. This must be defined by each module
type Module struct {
	name string

	// Moduler implementation
	moduler Moduler

	// Module config
	Config ModuleConfig

	// Raw config object to be unpacked by moduler
	cfg *ucfg.Config

	// List of all metricsets in this module. Use to keep track of metricsets
	metricSets map[string]*MetricSet

	Publish chan common.MapStr

	// MetricSet waitgroup
	wg sync.WaitGroup

	done chan struct{}
}

// NewModule creates a new module
func NewModule(cfg *ucfg.Config, moduler func() Moduler) (*Module, error) {

	// Module config defaults
	config := ModuleConfig{
		Period: "1s",
	}

	err := cfg.Unpack(&config)
	if err != nil {
		return nil, err
	}

	return &Module{
		name:       config.Module,
		Config:     config,
		cfg:        cfg,
		moduler:    moduler(),
		metricSets: map[string]*MetricSet{},
		// TODO: What should be size of channel?
		Publish: make(chan common.MapStr),
		wg:      sync.WaitGroup{},
		done:    make(chan struct{}),
	}, nil
}

// Starts the given module
func (m *Module) Start(b *beat.Beat) error {

	defer logp.Recover(fmt.Sprintf("Module %s paniced and stopped running.", m.name))

	if !m.Config.Enabled {
		logp.Debug("helper", "Not starting module %s with metricsets %s as not enabled.", m.name, m.getMetricSetsList())
		return nil
	}

	logp.Info("Setup moduler: %s", m.name)
	err := m.moduler.Setup(m.cfg)
	if err != nil {
		return fmt.Errorf("Error setting up module: %s. Not starting metricsets for this module.", err)
	}

	err = m.loadMetricsets()
	if err != nil {
		return fmt.Errorf("Error loading metricsets: %s", err)
	}

	// Setup period
	period, err := time.ParseDuration(m.Config.Period)
	if err != nil {
		return fmt.Errorf("Error in parsing period of metric %s: %v", m.name, err)
	}

	// If no period set, set default
	if period == 0 {
		logp.Info("Setting default period for metric %s as not set.", m.name)
		period = 1 * time.Second
	}

	// TODO: Improve logging information with list (names of metricSets)
	logp.Info("Start Module %s with metricsets [%s] and period %v", m.name, m.getMetricSetsList(), period)

	m.setupMetricSets()

	go m.Run(period, b)

	return nil
}

func (m *Module) setupMetricSets() {
	for _, set := range m.metricSets {
		err := set.Setup()
		if err != nil {
			logp.Err("Error setting up MetricSet %s: %s", set.Name, err)
		}
	}
}

func (m *Module) Run(period time.Duration, b *beat.Beat) {
	ticker := time.NewTicker(period)

	defer func() {
		logp.Info("Stopped module %s with metricsets %s", m.name, m.getMetricSetsList())
		ticker.Stop()
	}()

	var wg sync.WaitGroup
	ch := make(chan struct{})

	wait := func() {
		wg.Wait()
		ch <- struct{}{}
	}

	// TODO: A fetch event should take a maximum until the next ticker and
	// be stopped before the next request is sent. If a fetch is not successful
	// until the next it means it is a failure and a "error" event should be sent to es
	fetch := func(set *MetricSet) {
		defer wg.Done()
		// Move execution part to module?
		m.FetchMetricSets(set)
	}

	// Start publisher
	go m.publishing(b)

	// Start fetching metrics
	for {
		// Waits for next ticker
		select {
		case <-m.done:
			return
		case <-ticker.C:
		}

		for _, set := range m.metricSets {
			wg.Add(1)
			go fetch(set)
		}
		go wait()

		// Waits until all fetches are finished
		select {
		case <-m.done:
			return
		case <-ch:
			// finished parallel fetch
		}
	}
}

func (m *Module) FetchMetricSets(metricSet *MetricSet) {

	// Separate defer call as is has to be called directly
	defer logp.Recover(fmt.Sprintf("Metric %s paniced and stopped running.", m.name))

	err := metricSet.Fetch()
	if err != nil {
		logp.Err("Fetching events for MetricSet %s in Module %s returned error: %s", metricSet.Name, m.name, err)
	}
}

// Stop stops module and all its metricSets
func (m *Module) Stop() {
	logp.Info("Stopping module: %v", m.name)
	m.wg.Wait()
}

// loadMetricsets creates and setups the metricseter for the module
func (m *Module) loadMetricsets() error {
	// Setup - Create metricSets for the module
	for _, metricsetName := range m.Config.MetricSets {

		metricSet, err := Registry.GetMetricSet(m, metricsetName)
		if err != nil {
			return err
		}
		m.metricSets[metricsetName] = metricSet
	}
	return nil
}

// getMetricSetsList is a helper function that returns a list of all module metricsets as string
// This is mostly used for logging
func (m *Module) getMetricSetsList() string {

	// Sort list first alphabetically
	keys := make([]string, 0, len(m.metricSets))
	for key := range m.metricSets {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	// Create output string
	list := ""
	first := true

	for _, value := range keys {
		if !first {
			list = list + ", "
		}
		first = false
		list = list + value
	}

	return list
}

// publishing runs the receiving channel to receive events from the metricset
// and forward them to the publisher
func (m *Module) publishing(b *beat.Beat) {
	for {

		select {
		case <-m.done:
			return
		case event := <-m.Publish:
			// TODO transform to publish events - @ruflin,20160314
			// Will this merge multiple events together to use bulk sending?
			b.Events.PublishEvent(event)
		}
	}
}

// ProcessConfig allows to process additional configuration params which are not
// part of the module default configuratoin. This allows each metricset
// to have its specific config params
func (m *Module) ProcessConfig(config interface{}) error {

	if err := m.cfg.Unpack(config); err != nil {
		return err
	}
	return nil
}
