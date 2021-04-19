package main

import (
	"math"
	"net/http"

	"github.com/golang/glog"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	dto "github.com/prometheus/client_model/go"
)

const (
	namespace = "vtk"
)

var (
	syncTime = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "sync_time",
		Help:      "How long the sync run took",
	})
	syncCount = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "sync_count",
		Help:      "How many times sync was running since application start",
	})
	syncStatus = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "sync_status",
		Help:      "Status of sync",
	},
		[]string{"namespace"},
	)
	secretsCreated = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "secrets_created",
		Help:      "How many secrets were created in k8s during sync cycle",
	},
		[]string{"namespace"},
	)
	secretsUpdated = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "secrets_updated",
		Help:      "How many secrets were updated in k8s during sync cycle",
	},
		[]string{"namespace"},
	)
	secretsSkipped = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "secrets_skipped",
		Help:      "How many secrets were skipped during sync cycle",
	},
		[]string{"namespace"},
	)
	secretsSynced = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "secrets_synced",
		Help:      "How many secrets were synced during sync cycle",
	},
		[]string{"namespace"},
	)
	authApproleSecretID = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "auth_approle_secret_id",
		Help:      "AppRole Secret ID rotation info",
	},
		[]string{"type"},
	)
	authToken = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "auth_token",
		Help:      "Token rotation info",
	},
		[]string{"type"},
	)
)

func prometheusMetricsFunc() {
	http.Handle(prometheusMetricsPath, promhttp.Handler())
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html>
			<head><title>Vault to K8s Prometheus Exporter</title></head>
			<body>
			<h1>Vault to K8s Prometheus Exporter</h1>
			<p><a href="` + prometheusMetricsPath + `">Metrics</a></p>
			</body>
			</html>`))
	})

	prometheus.MustRegister(syncTime)
	prometheus.MustRegister(syncCount)
	prometheus.MustRegister(syncStatus)
	prometheus.MustRegister(secretsCreated)
	prometheus.MustRegister(secretsUpdated)
	prometheus.MustRegister(secretsSkipped)
	prometheus.MustRegister(secretsSynced)
	prometheus.MustRegister(authApproleSecretID)
	prometheus.MustRegister(authToken)

	glog.Infoln("Prometheus exporter enabled")
	glog.Infoln("Prometheus exporter metrics path", prometheusMetricsPath)
	glog.Infoln("Prometheus exporter listening on", prometheusListenAddress)
	if err := http.ListenAndServe(prometheusListenAddress, nil); err != nil {
		glog.Fatal(err)
	}
}

// https://github.com/prometheus/client_golang/issues/412
func readMetricValue(m prometheus.Metric) float64 {
	pb := &dto.Metric{}
	m.Write(pb)
	if pb.Gauge != nil {
		return pb.Gauge.GetValue()
	}
	if pb.Counter != nil {
		return pb.Counter.GetValue()
	}
	return math.NaN()
}
