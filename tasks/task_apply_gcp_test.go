package tasks

import (
	"context"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/Releem/mysqlconfigurer/config"
	"github.com/Releem/mysqlconfigurer/models"
	logging "github.com/google/logger"
	"google.golang.org/api/option"
	"google.golang.org/api/sqladmin/v1"
)

func TestFilterGCPRecommendationsByApplyMode(t *testing.T) {
	recommendations := models.MetricGroupValue{
		"dynamic_flag": "1",
		"restart_flag": "2",
		"unknown_flag": "3",
	}
	flags := []*sqladmin.Flag{
		{Name: "dynamic_flag", RequiresRestart: false},
		{Name: "restart_flag", RequiresRestart: true},
	}

	dynamic, unknown := filterGCPRecommendationsByApplyMode(recommendations, flags, "dynamic")
	if want := (models.MetricGroupValue{"dynamic_flag": "1"}); !reflect.DeepEqual(dynamic, want) {
		t.Errorf("filterGCPRecommendationsByApplyMode(dynamic) = %v, want %v", dynamic, want)
	}
	if want := []string{"unknown_flag"}; !reflect.DeepEqual(unknown, want) {
		t.Errorf("filterGCPRecommendationsByApplyMode(dynamic) unknown = %v, want %v", unknown, want)
	}

	full, unknown := filterGCPRecommendationsByApplyMode(recommendations, flags, "full")
	if !reflect.DeepEqual(full, recommendations) {
		t.Errorf("filterGCPRecommendationsByApplyMode(full) = %v, want %v", full, recommendations)
	}
	if len(unknown) != 0 {
		t.Errorf("filterGCPRecommendationsByApplyMode(full) unknown = %v, want none", unknown)
	}
}

func TestGCPApplyOperationFailureReturnsTaskResult(t *testing.T) {
	client := &http.Client{Transport: gcpApplyTransport(func(r *http.Request) (*http.Response, error) {
		body := `{"name":"db","databaseVersion":"MYSQL_8_0","settings":{}}`
		switch {
		case strings.Contains(r.URL.Path, "/operations/"):
			body = `{"status":"DONE","error":{"errors":[{"message":"operation rejected"}]}}`
		case r.Method == http.MethodPatch:
			body = `{"name":"apply-op"}`
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})}
	previous := newGCPApplyService
	t.Cleanup(func() { newGCPApplyService = previous })
	newGCPApplyService = func(ctx context.Context) (*sqladmin.Service, error) {
		return sqladmin.NewService(ctx, option.WithHTTPClient(client), option.WithoutAuthentication())
	}
	logger := logging.Init("gcp-apply-test", false, false, io.Discard)
	defer logger.Close()
	code, status, output := ApplyConfGcpCloudSQL(&awsApplyRepeater{recommendations: `{"max_connections":"200"}`}, []models.MetricsGatherer{&awsApplyGatherer{current: map[string]interface{}{"max_connections": "100"}}}, *logger, &config.Config{GcpProjectId: "test", GcpCloudSqlInstance: "db"}, "full")
	if code != 8 || status != 4 || !strings.Contains(output, "operation rejected") {
		t.Fatalf("ApplyConfGcpCloudSQL = %d/%d %q, want 8/4 and operation error", code, status, output)
	}
}

type gcpApplyTransport func(*http.Request) (*http.Response, error)

func (f gcpApplyTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
