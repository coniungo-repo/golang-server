// Package metrics holds the API's Prometheus metrics in a private registry, served by Handler on a port separate from the API. Every method is safe on a nil *Metrics, which records nothing.
package metrics

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/coniungo-repo/golang-server/internal/platform/config"
	"github.com/coniungo-repo/golang-server/internal/platform/email"
	"github.com/coniungo-repo/golang-server/internal/platform/sms"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	ResultOK    = "ok"
	ResultError = "error"

	LimitClientIP = "client_ip"
	LimitUser     = "user"
	LimitCodeSend = "code_send"
)

var knownMethods = map[string]bool{
	http.MethodGet: true, http.MethodHead: true, http.MethodPost: true, http.MethodPut: true,
	http.MethodPatch: true, http.MethodDelete: true, http.MethodOptions: true,
}

type Metrics struct {
	registry         *prometheus.Registry
	httpRequests     *prometheus.CounterVec
	httpDuration     *prometheus.HistogramVec
	rateLimited      *prometheus.CounterVec
	deliveries       *prometheus.CounterVec
	deliveryDuration *prometheus.HistogramVec
	coreRequests     *prometheus.CounterVec
	coreDuration     *prometheus.HistogramVec
	smsTotalCounted  prometheus.Counter
	smsTotalLimit    *prometheus.GaugeVec
}

// New registers the API metrics plus Go runtime and process metrics. version is exposed as app_build_info.
func New(version string) *Metrics {
	m := &Metrics{
		registry: prometheus.NewRegistry(),
		httpRequests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "http_requests_total", Help: "Requests by method, matched route pattern and status code.",
		}, []string{"method", "route", "status"}),
		httpDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "http_request_duration_seconds", Help: "Time to serve a request, by method and matched route pattern.",
			Buckets: prometheus.DefBuckets,
		}, []string{"method", "route"}),
		rateLimited: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "rate_limited_total", Help: "Requests refused by a rate limit: client_ip, user or code_send.",
		}, []string{"limit"}),
		deliveries: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "code_deliveries_total", Help: "Sign-in code sends by channel (email, sms) and result (ok, error).",
		}, []string{"channel", "result"}),
		deliveryDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "code_delivery_duration_seconds", Help: "Time the email or SMS provider took to accept a code.",
			Buckets: []float64{.1, .25, .5, 1, 2, 5, 10},
		}, []string{"channel"}),
		coreRequests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "supertokens_core_requests_total", Help: "Calls to the SuperTokens core by HTTP status code.",
		}, []string{"code"}),
		smsTotalCounted: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "sms_total_counted_total", Help: "Texts to numbers without an account, which count toward AUTH_RATE_LIMIT_SMS_TOTAL.",
		}),
		smsTotalLimit: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "sms_total_limit", Help: "The AUTH_RATE_LIMIT_SMS_TOTAL ceiling for each window, for alerts on how close texts are to it.",
		}, []string{"window"}),
		coreDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "supertokens_core_request_duration_seconds", Help: "Time each call to the SuperTokens core took.",
			Buckets: prometheus.DefBuckets,
		}, []string{}),
	}
	build := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "app_build_info", Help: "Always 1, labelled with the running version.", ConstLabels: prometheus.Labels{"version": version},
	})
	build.Set(1)
	m.registry.MustRegister(
		m.httpRequests, m.httpDuration, m.rateLimited, m.deliveries, m.deliveryDuration, m.coreRequests, m.coreDuration,
		m.smsTotalCounted, m.smsTotalLimit, build,
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)
	return m
}

// Handler serves the metrics in the Prometheus text format.
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}

// ObserveRequest records one served request. route is the matched pattern, never the raw path, so labels stay bounded.
func (m *Metrics) ObserveRequest(method, route string, status int, d time.Duration) {
	if m == nil {
		return
	}
	if !knownMethods[method] {
		method = "OTHER"
	}
	m.httpRequests.WithLabelValues(method, route, strconv.Itoa(status)).Inc()
	m.httpDuration.WithLabelValues(method, route).Observe(d.Seconds())
}

// RateLimited counts a request refused by limit.
func (m *Metrics) RateLimited(limit string) {
	if m == nil {
		return
	}
	m.rateLimited.WithLabelValues(limit).Inc()
}

// SMSTotalCounted counts a text that counted toward the SMS total.
func (m *Metrics) SMSTotalCounted() {
	if m == nil {
		return
	}
	m.smsTotalCounted.Inc()
}

// SetSMSTotalLimits publishes the SMS total's ceilings, labelled by window such as 1h or 24h.
func (m *Metrics) SetSMSTotalLimits(windows []config.RateWindow) {
	if m == nil {
		return
	}
	for _, w := range windows {
		m.smsTotalLimit.WithLabelValues(windowLabel(w.Period)).Set(float64(w.Limit))
	}
}

func windowLabel(d time.Duration) string {
	switch {
	case d%time.Hour == 0:
		return strconv.Itoa(int(d/time.Hour)) + "h"
	case d%time.Minute == 0:
		return strconv.Itoa(int(d/time.Minute)) + "m"
	default:
		return strconv.Itoa(int(d/time.Second)) + "s"
	}
}

// CoreTransport wraps next so every call to the SuperTokens core is counted and timed.
func (m *Metrics) CoreTransport(next http.RoundTripper) http.RoundTripper {
	if m == nil {
		return next
	}
	return promhttp.InstrumentRoundTripperCounter(m.coreRequests, promhttp.InstrumentRoundTripperDuration(m.coreDuration, next))
}

// RegisterPool exposes the database pool's connections and waits, read on each scrape.
func (m *Metrics) RegisterPool(pool *pgxpool.Pool) {
	if m == nil {
		return
	}
	m.registry.MustRegister(poolCollector{pool: pool})
}

// Email wraps s so each send is counted and timed under the email channel.
func (m *Metrics) Email(s email.Sender) email.Sender {
	if m == nil {
		return s
	}
	return emailSender{next: s, m: m}
}

// SMS wraps s so each send is counted and timed under the sms channel.
func (m *Metrics) SMS(s sms.Sender) sms.Sender {
	if m == nil {
		return s
	}
	return smsSender{next: s, m: m}
}

func (m *Metrics) observeDelivery(channel string, start time.Time, err error) {
	result := ResultOK
	if err != nil {
		result = ResultError
	}
	m.deliveries.WithLabelValues(channel, result).Inc()
	m.deliveryDuration.WithLabelValues(channel).Observe(time.Since(start).Seconds())
}

type emailSender struct {
	next email.Sender
	m    *Metrics
}

func (s emailSender) Send(ctx context.Context, msg email.Message) error {
	start := time.Now()
	err := s.next.Send(ctx, msg)
	s.m.observeDelivery("email", start, err)
	return err
}

type smsSender struct {
	next sms.Sender
	m    *Metrics
}

func (s smsSender) Send(ctx context.Context, msg sms.Message) error {
	start := time.Now()
	err := s.next.Send(ctx, msg)
	s.m.observeDelivery("sms", start, err)
	return err
}
