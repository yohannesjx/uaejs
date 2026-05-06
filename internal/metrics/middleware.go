package metrics

import (
	"net/http"
	"strconv"
	"time"
)

// responseWriter wraps http.ResponseWriter to capture the status code.
type responseWriter struct {
	http.ResponseWriter
	status int
	written bool
}

func (rw *responseWriter) WriteHeader(code int) {
	if !rw.written {
		rw.status = code
		rw.written = true
	}
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	if !rw.written {
		rw.WriteHeader(http.StatusOK)
	}
	return rw.ResponseWriter.Write(b)
}

// HTTPMiddleware returns a chi-compatible middleware that records request
// count and latency via the provided Metrics instance.
//
// Usage:
//
//	r.Use(m.HTTPMiddleware)
func (m *Metrics) HTTPMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(rw, r)

		duration := time.Since(start).Seconds()
		path := r.URL.Path
		method := r.Method
		status := strconv.Itoa(rw.status)

		m.HTTPRequestsTotal.WithLabelValues(method, path, status).Inc()
		m.HTTPRequestDuration.WithLabelValues(method, path).Observe(duration)
	})
}

// ObservePendingInvoices is a background collector helper called by the
// low-stock Asynq worker to refresh the pending-invoices gauge.
func (m *Metrics) ObservePendingInvoices(count float64) {
	m.PendingInvoices.Set(count)
}

// ObserveActiveReservations refreshes the active-reservations gauge.
func (m *Metrics) ObserveActiveReservations(count float64) {
	m.ActiveReservations.Set(count)
}
