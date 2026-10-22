package metrics

import (
	"io"
	"testing"

	"github.com/Releem/mysqlconfigurer/config"
	"github.com/Releem/mysqlconfigurer/models"
	logging "github.com/google/logger"
)

func TestOneShotQueryOptimizationCollectsSamplesFirst(t *testing.T) {
	logger := logging.Init("one-shot-samples", false, false, io.Discard)
	defer logger.Close()
	originalSamples := models.SampleQueries
	t.Cleanup(func() { models.SampleQueries = originalSamples })
	for _, tc := range []struct {
		name        string
		mode        models.ModeType
		enabled     bool
		wantSamples int
	}{
		{"query_optimization", models.ModeType{Name: "TaskByName", Type: "queries_optimization"}, true, 1},
		{"disabled", models.ModeType{Name: "TaskByName", Type: "queries_optimization"}, false, 0},
		{"configuration_download", models.ModeType{Name: "Configurations", Type: "Get"}, true, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			models.SampleQueries = make(map[string]string)
			sampleCalls, queryCalls := 0, 0
			queryText := ""
			configuration := &config.Config{PgUser: "test", PgPassword: "test", QueryOptimization: tc.enabled}
			gatherers := map[string][]models.MetricsGatherer{
				"sample_queries": {sampleOrderGatherer(func(*models.Metrics) error {
					sampleCalls++
					models.SampleQueriesMutex.Lock()
					models.SampleQueries["appdigest"] = "SELECT name FROM orders"
					models.SampleQueriesMutex.Unlock()
					return nil
				})},
				"query_optimization": {sampleOrderGatherer(func(*models.Metrics) error {
					queryCalls++
					models.SampleQueriesMutex.RLock()
					queryText = models.SampleQueries["appdigest"]
					models.SampleQueriesMutex.RUnlock()
					return nil
				})},
			}
			if err := collectConfiguration(gatherers, sampleOrderRepeater{}, *logger, configuration, tc.mode); err != nil {
				t.Fatalf("collectConfiguration(%s) = %v, want nil", tc.name, err)
			}
			if sampleCalls != tc.wantSamples {
				t.Errorf("collectConfiguration(%s) sample calls = %d, want %d", tc.name, sampleCalls, tc.wantSamples)
			}
			if tc.mode.Name == "TaskByName" && queryCalls != 1 {
				t.Errorf("collectConfiguration(%s) query calls = %d, want 1", tc.name, queryCalls)
			}
			if tc.wantSamples == 1 && queryText != "SELECT name FROM orders" {
				t.Errorf("collectConfiguration(%s) fallback query text = %q, want collected sample", tc.name, queryText)
			}
		})
	}
}

type sampleOrderGatherer func(*models.Metrics) error

func (g sampleOrderGatherer) GetMetrics(metrics *models.Metrics) error { return g(metrics) }

type sampleOrderRepeater struct{}

func (sampleOrderRepeater) ProcessMetrics(models.MetricContext, models.Metrics, models.ModeType) (string, error) {
	return "ok", nil
}
