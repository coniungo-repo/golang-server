package metrics

import (
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	poolConns        = prometheus.NewDesc("db_pool_connections", "Connections in the pool by state.", []string{"state"}, nil)
	poolMax          = prometheus.NewDesc("db_pool_max_connections", "The pool's maximum size, DB_MAX_CONNS.", nil, nil)
	poolAcquires     = prometheus.NewDesc("db_pool_acquires_total", "Connections handed out.", nil, nil)
	poolEmpty        = prometheus.NewDesc("db_pool_empty_acquires_total", "Acquires that had to wait because no connection was idle.", nil, nil)
	poolCanceled     = prometheus.NewDesc("db_pool_canceled_acquires_total", "Acquires abandoned because the request context ended first.", nil, nil)
	poolWaitDuration = prometheus.NewDesc("db_pool_acquire_wait_seconds_total", "Total time spent acquiring connections.", nil, nil)
)

type poolCollector struct {
	pool *pgxpool.Pool
}

func (c poolCollector) Describe(ch chan<- *prometheus.Desc) {
	for _, d := range []*prometheus.Desc{poolConns, poolMax, poolAcquires, poolEmpty, poolCanceled, poolWaitDuration} {
		ch <- d
	}
}

func (c poolCollector) Collect(ch chan<- prometheus.Metric) {
	s := c.pool.Stat()
	ch <- prometheus.MustNewConstMetric(poolConns, prometheus.GaugeValue, float64(s.AcquiredConns()), "acquired")
	ch <- prometheus.MustNewConstMetric(poolConns, prometheus.GaugeValue, float64(s.IdleConns()), "idle")
	ch <- prometheus.MustNewConstMetric(poolConns, prometheus.GaugeValue, float64(s.ConstructingConns()), "constructing")
	ch <- prometheus.MustNewConstMetric(poolMax, prometheus.GaugeValue, float64(s.MaxConns()))
	ch <- prometheus.MustNewConstMetric(poolAcquires, prometheus.CounterValue, float64(s.AcquireCount()))
	ch <- prometheus.MustNewConstMetric(poolEmpty, prometheus.CounterValue, float64(s.EmptyAcquireCount()))
	ch <- prometheus.MustNewConstMetric(poolCanceled, prometheus.CounterValue, float64(s.CanceledAcquireCount()))
	ch <- prometheus.MustNewConstMetric(poolWaitDuration, prometheus.CounterValue, s.AcquireDuration().Seconds())
}
