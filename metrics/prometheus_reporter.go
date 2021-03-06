// Copyright (c) TFG Co. All Rights Reserved.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package metrics

import (
	"fmt"
	"log"
	"net/http"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Summary defines a summary metric
type Summary struct {
	Subsystem  string
	Name       string
	Help       string
	Objectives map[float64]float64
	Labels     []string
}

// Gauge defines a gauge metric
type Gauge struct {
	Subsystem string
	Name      string
	Help      string
	Labels    []string
}

// Counter defines a counter metric
type Counter struct {
	Subsystem string
	Name      string
	Help      string
	Labels    []string
}

var (
	prometheusReporter *PrometheusReporter
	once               sync.Once
)

// PrometheusReporter reports metrics to prometheus
type PrometheusReporter struct {
	serverType          string
	game                string
	countReportersMap   map[string]*prometheus.CounterVec
	summaryReportersMap map[string]*prometheus.SummaryVec
	gaugeReportersMap   map[string]*prometheus.GaugeVec
	additionalLabels    map[string]string
}

func (p *PrometheusReporter) registerMetrics(
	constLabels, additionalLabels map[string]string,
) {

	constLabels["game"] = p.game
	constLabels["serverType"] = p.serverType

	p.additionalLabels = additionalLabels
	additionalLabelsKeys := make([]string, 0, len(additionalLabels))
	for key := range additionalLabels {
		additionalLabelsKeys = append(additionalLabelsKeys, key)
	}

	// HandlerResponseTimeMs summary
	p.summaryReportersMap[ResponseTime] = prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Namespace:   "nano",
			Subsystem:   "handler",
			Name:        ResponseTime,
			Help:        "the time to process a msg in nanoseconds",
			Objectives:  map[float64]float64{0.7: 0.02, 0.95: 0.005, 0.99: 0.001},
			ConstLabels: constLabels,
		},
		append([]string{"route"}, additionalLabelsKeys...),
	)

	// ProcessDelay summary
	p.summaryReportersMap[ProcessDelay] = prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Namespace:   "nano",
			Subsystem:   "handler",
			Name:        ProcessDelay,
			Help:        "the delay to start processing a msg in nanoseconds",
			Objectives:  map[float64]float64{0.7: 0.02, 0.95: 0.005, 0.99: 0.001},
			ConstLabels: constLabels,
		},
		append([]string{"route"}, additionalLabelsKeys...),
	)

	// ConnectedClients gauge
	p.gaugeReportersMap[ConnectedClients] = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace:   "nano",
			Subsystem:   "acceptor",
			Name:        ConnectedClients,
			Help:        "the number of clients connected right now",
			ConstLabels: constLabels,
		},
		additionalLabelsKeys,
	)

	p.gaugeReportersMap[Goroutines] = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace:   "nano",
			Subsystem:   "sys",
			Name:        Goroutines,
			Help:        "the current number of goroutines",
			ConstLabels: constLabels,
		},
		additionalLabelsKeys,
	)

	p.gaugeReportersMap[HeapSize] = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace:   "nano",
			Subsystem:   "sys",
			Name:        HeapSize,
			Help:        "the current heap size",
			ConstLabels: constLabels,
		},
		additionalLabelsKeys,
	)

	p.gaugeReportersMap[HeapObjects] = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace:   "nano",
			Subsystem:   "sys",
			Name:        HeapObjects,
			Help:        "the current number of allocated heap objects",
			ConstLabels: constLabels,
		},
		additionalLabelsKeys,
	)

	p.gaugeReportersMap[MessageCount] = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace:   "nano",
			Subsystem:   "acceptor",
			Name:        MessageCount,
			Help:        "the current number of processed message",
			ConstLabels: constLabels,
		},
		additionalLabelsKeys,
	)

	p.countReportersMap[ExceededRateLimiting] = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace:   "nano",
			Subsystem:   "acceptor",
			Name:        ExceededRateLimiting,
			Help:        "the number of blocked requests by exceeded rate limiting",
			ConstLabels: constLabels,
		},
		additionalLabelsKeys,
	)

	toRegister := make([]prometheus.Collector, 0)
	for _, c := range p.countReportersMap {
		toRegister = append(toRegister, c)
	}

	for _, c := range p.gaugeReportersMap {
		toRegister = append(toRegister, c)
	}

	for _, c := range p.summaryReportersMap {
		toRegister = append(toRegister, c)
	}

	prometheus.MustRegister(toRegister...)
}

// GetPrometheusReporter gets the prometheus reporter singleton
func GetPrometheusReporter(
	port int,
	game string,
	serverType string,
	constLabels map[string]string,
) (*PrometheusReporter, error) {
	var (
		additionalLabels = make(map[string]string)
	)

	once.Do(func() {
		prometheusReporter = &PrometheusReporter{
			serverType:          serverType,
			game:                game,
			countReportersMap:   make(map[string]*prometheus.CounterVec),
			summaryReportersMap: make(map[string]*prometheus.SummaryVec),
			gaugeReportersMap:   make(map[string]*prometheus.GaugeVec),
		}
		prometheusReporter.registerMetrics(constLabels, additionalLabels)
		http.Handle("/metrics", promhttp.Handler())
		go (func() {
			log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", port), nil))
		})()
	})

	return prometheusReporter, nil
}

// ReportSummary reports a summary metric
func (p *PrometheusReporter) ReportSummary(metric string, labels map[string]string, value float64) error {
	sum := p.summaryReportersMap[metric]
	if sum != nil {
		labels = p.ensureLabels(labels)
		sum.With(labels).Observe(value)
		return nil
	}
	return nil
}

// ReportCount reports a summary metric
func (p *PrometheusReporter) ReportCount(metric string, labels map[string]string, count float64) error {
	cnt := p.countReportersMap[metric]
	if cnt != nil {
		labels = p.ensureLabels(labels)
		cnt.With(labels).Add(count)
		return nil
	}
	return nil
}

// ReportGauge reports a gauge metric
func (p *PrometheusReporter) ReportGauge(metric string, labels map[string]string, value float64) error {
	g := p.gaugeReportersMap[metric]
	if g != nil {
		labels = p.ensureLabels(labels)
		g.With(labels).Set(value)
		return nil
	}
	return nil
}

// ensureLabels checks if labels contains the additionalLabels values,
// otherwise adds them with the default values
func (p *PrometheusReporter) ensureLabels(labels map[string]string) map[string]string {
	for key, defaultVal := range p.additionalLabels {
		if _, ok := labels[key]; !ok {
			labels[key] = defaultVal
		}
	}

	return labels
}
