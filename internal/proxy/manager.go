package proxy

import (
	"context"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.uber.org/zap"
)

type HealthcheckManagerConfig struct {
	Targets []NodeProviderConfig
	Config  HealthCheckConfig
}

type HealthcheckManager struct {
	healthcheckers []Healthchecker

	metricRPCProviderInfo        *prometheus.GaugeVec
	metricRPCProviderStatus      *prometheus.GaugeVec
	metricRPCProviderBlockNumber *prometheus.GaugeVec
	metricRPCProviderGasLimit    *prometheus.GaugeVec
}

func NewHealthcheckManager(config HealthcheckManagerConfig) *HealthcheckManager {
	healthCheckers := []Healthchecker{}

	healthcheckManager := &HealthcheckManager{
		metricRPCProviderInfo: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "zeroex_rpc_gateway_provider_info",
				Help: "Gas limit of a given provider",
			}, []string{
				"index",
				"provider",
			}),
		metricRPCProviderStatus: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "zeroex_rpc_gateway_provider_status",
				Help: "Current status of a given provider by type. Type can be either healthy or tainted.",
			}, []string{
				"provider",
				"type",
			}),
		metricRPCProviderBlockNumber: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "zeroex_rpc_gateway_provider_block_number",
				Help: "Block number of a given provider",
			}, []string{
				"provider",
			}),
		metricRPCProviderGasLimit: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "zeroex_rpc_gateway_provider_gasLimit_number",
				Help: "Gas limit of a given provider",
			}, []string{
				"provider",
			}),
	}

	for _, target := range config.Targets {
		healthchecker, err := NewHealthchecker(
			RPCHealthcheckerConfig{
				URL:              target.Connection.HTTP.URL,
				Name:             target.Name,
				Interval:         config.Config.Interval,
				Timeout:          config.Config.Timeout,
				FailureThreshold: config.Config.FailureThreshold,
				SuccessThreshold: config.Config.SuccessThreshold,
			})

		if err != nil {
			panic(err)
		}

		healthCheckers = append(healthCheckers, healthchecker)
	}

	healthcheckManager.healthcheckers = healthCheckers

	return healthcheckManager
}

func (h *HealthcheckManager) runLoop(ctx context.Context) error {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			h.reportStatusMetrics()
		}
	}
}

func (h *HealthcheckManager) reportStatusMetrics() {
	for _, healthchecker := range h.healthcheckers {
		healthy := 0
		tainted := 0
		if healthchecker.IsHealthy() {
			healthy = 1
		}
		if healthchecker.IsTainted() {
			tainted = 1
		}
		h.metricRPCProviderGasLimit.WithLabelValues(healthchecker.Name()).Set(float64(healthchecker.BlockNumber()))
		h.metricRPCProviderBlockNumber.WithLabelValues(healthchecker.Name()).Set(float64(healthchecker.BlockNumber()))
		h.metricRPCProviderStatus.WithLabelValues(healthchecker.Name(), "healthy").Set(float64(healthy))
		h.metricRPCProviderStatus.WithLabelValues(healthchecker.Name(), "tainted").Set(float64(tainted))
	}
}

func (h *HealthcheckManager) Start(ctx context.Context) error {
	for index, healthChecker := range h.healthcheckers {
		h.metricRPCProviderInfo.WithLabelValues(strconv.Itoa(index), healthChecker.Name()).Set(1)
		go healthChecker.Start(ctx)
	}

	return h.runLoop(ctx)
}

func (h *HealthcheckManager) Stop(ctx context.Context) error {
	for _, healthChecker := range h.healthcheckers {
		err := healthChecker.Stop(ctx)
		if err != nil {
			zap.L().Error("healtchecker stop error", zap.Error(err))
		}
	}

	return nil
}

func (h *HealthcheckManager) GetTargetByName(name string) Healthchecker {
	for _, healthChecker := range h.healthcheckers {
		if healthChecker.Name() == name {
			return healthChecker
		}
	}

	zap.L().Error("tried to access a non-existing Healthchecker", zap.String("name", name))
	return nil
}

func (h *HealthcheckManager) TaintTarget(name string) {
	if healthChecker := h.GetTargetByName(name); healthChecker != nil {
		healthChecker.Taint()
		return
	}
}
