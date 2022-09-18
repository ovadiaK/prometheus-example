package middleware

import (
	"fmt"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	io_prometheus_client "github.com/prometheus/client_model/go"
	"net/http"
	"regexp"
	"strconv"
	"strings"
)

type Prom struct {
	reg            *prometheus.Registry
	totalRequests  *prometheus.CounterVec
	responseStatus *prometheus.CounterVec
	httpDuration   *prometheus.HistogramVec
}

var _ = prometheus.Gatherer(&Prom{})

func (p *Prom) Gather() ([]*io_prometheus_client.MetricFamily, error) {
	return p.reg.Gather()
}

func New(serviceName string) (*Prom, func(handler http.Handler) http.Handler) {
	rec := &Prom{
		reg: prometheus.NewRegistry(),
	}
	rec.totalRequests = promauto.With(rec.reg).NewCounterVec(
		prometheus.CounterOpts{
			Name: fmt.Sprintf("%s_http_requests_total", toSnakeCase(serviceName)),
			Help: fmt.Sprintf("Number of requests to service %s according to path.", serviceName),
		},
		[]string{"path"},
	)
	rec.responseStatus = promauto.With(rec.reg).NewCounterVec(
		prometheus.CounterOpts{
			Name: fmt.Sprintf("%s_response_status", toSnakeCase(serviceName)),
			Help: fmt.Sprintf("Status of HTTP response for service %s", serviceName),
		},
		[]string{"status"},
	)
	rec.httpDuration = promauto.With(rec.reg).NewHistogramVec(prometheus.HistogramOpts{
		Name: fmt.Sprintf("%s_http_response_time_seconds", toSnakeCase(serviceName)),
		Help: fmt.Sprintf("Duration of HTTP requests for service %s according to path.", serviceName),
	}, []string{"path"})
	middleware := makeMiddleware(rec)
	return rec, middleware
}

var (
	matchFirstCap = regexp.MustCompile("(.)([A-Z][a-z]+)")
	matchAllCap   = regexp.MustCompile("([a-z0-9])([A-Z])")
)

func toSnakeCase(str string) string {
	snake := matchFirstCap.ReplaceAllString(str, "${1}_${2}")
	snake = matchAllCap.ReplaceAllString(snake, "${1}_${2}")
	return strings.ToLower(snake)
}

type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func newResponseWriter(w http.ResponseWriter) *responseWriter {
	return &responseWriter{w, http.StatusOK}
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}
func makeMiddleware(rec *Prom) func(handler http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			path := r.RequestURI
			timer := prometheus.NewTimer(rec.httpDuration.WithLabelValues(path))
			rw := newResponseWriter(w)
			next.ServeHTTP(rw, r)

			statusCode := rw.statusCode

			rec.responseStatus.WithLabelValues(strconv.Itoa(statusCode)).Inc()
			rec.totalRequests.WithLabelValues(path).Inc()

			timer.ObserveDuration()
		})
	}
}
