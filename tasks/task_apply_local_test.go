package tasks

import (
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"reflect"
	"strings"
	"testing"
	"testing/synctest"

	"github.com/Releem/mysqlconfigurer/config"
	"github.com/Releem/mysqlconfigurer/models"
	logging "github.com/google/logger"
)

func TestLocalDynamicApplyQueries(t *testing.T) {
	tests := []struct {
		name            string
		databaseType    string
		recommendations models.MetricGroupValue
		current         models.MetricGroupValue
		want            []localDynamicApplyOperation
	}{
		{
			name:         "mysql sets only changed value",
			databaseType: "mysql",
			recommendations: models.MetricGroupValue{
				"max_connections":   "200",
				"thread_cache_size": "10",
			},
			current: models.MetricGroupValue{
				"max_connections":   "100",
				"thread_cache_size": "10",
			},
			want: []localDynamicApplyOperation{
				{query: "set global max_connections=200", parameterNames: []string{"max_connections"}},
			},
		},
		{
			name:         "postgres reloads once when dynamic value changed",
			databaseType: "postgresql",
			recommendations: models.MetricGroupValue{
				"work_mem":       "16MB",
				"shared_buffers": "256MB",
			},
			current: models.MetricGroupValue{
				"work_mem":       map[string]interface{}{"setting": "8MB"},
				"shared_buffers": map[string]interface{}{"setting": "128MB"},
			},
			want: []localDynamicApplyOperation{
				{
					query:          "SELECT pg_reload_conf()",
					parameterNames: []string{"shared_buffers", "work_mem"},
				},
			},
		},
		{
			name:         "postgres does not reload unchanged config",
			databaseType: "postgresql",
			recommendations: models.MetricGroupValue{
				"max_connections": "100",
			},
			current: models.MetricGroupValue{
				"max_connections": map[string]interface{}{"setting": "100"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := localDynamicApplyQueries(tt.databaseType, tt.recommendations, tt.current)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("localDynamicApplyQueries(%q, %v, %v) = %v, want %v", tt.databaseType, tt.recommendations, tt.current, got, tt.want)
			}
		})
	}
}

func TestApplyConfLocalIncludesLogsInTaskOutput(t *testing.T) {
	tests := []struct {
		name            string
		postgresql      bool
		recommendations string
		current         models.MetricGroupValue
		execErr         error
		wantQueries     []string
		wantExit        int
		wantStatus      int
		wantOutput      string
	}{
		{
			name:            "mysql changes and unchanged parameter",
			recommendations: `{"max_connections":"200","thread_cache_size":"10"}`,
			current:         models.MetricGroupValue{"max_connections": "100", "thread_cache_size": "10"},
			wantQueries:     []string{"set global max_connections=200"},
			wantStatus:      1,
			wantOutput:      "max_connections: 100 -> 200\nDynamic apply: changes_applied=true, restart_required=false, insufficient_privileges=false, errors=false\n",
		},
		{
			name:            "postgresql reload with multiple changes",
			postgresql:      true,
			recommendations: `{"work_mem":"16MB","shared_buffers":"256MB"}`,
			current:         models.MetricGroupValue{"work_mem": map[string]interface{}{"setting": "8MB"}, "shared_buffers": map[string]interface{}{"setting": "128MB"}},
			wantQueries:     []string{"SELECT pg_reload_conf()"},
			wantStatus:      1,
			wantOutput:      "shared_buffers: 128MB -> 256MB\nwork_mem: 8MB -> 16MB\nDynamic apply: changes_applied=true, restart_required=false, insufficient_privileges=false, errors=false\n",
		},
		{
			name:            "postgresql denied reload retains change and error logs",
			postgresql:      true,
			recommendations: `{"work_mem":"16MB"}`,
			current:         models.MetricGroupValue{"work_mem": map[string]interface{}{"setting": "8MB"}},
			execErr:         errors.New("permission denied for function pg_reload_conf"),
			wantQueries:     []string{"SELECT pg_reload_conf()"},
			wantExit:        9, wantStatus: 4,
			wantOutput: "work_mem: 8MB -> 16MB\nwork_mem: permission denied for function pg_reload_conf\nDynamic apply: changes_applied=false, restart_required=false, insufficient_privileges=true, errors=false\n",
		},
		{
			name:            "unchanged configuration reports no changes",
			recommendations: `{"max_connections":"100"}`,
			current:         models.MetricGroupValue{"max_connections": "100"},
			wantStatus:      1,
			wantOutput:      "Dynamic apply: changes_applied=false, restart_required=false, insufficient_privileges=false, errors=false\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				connector := &localApplyTestConnector{err: tt.execErr}
				db := sql.OpenDB(connector)
				defer db.Close()
				previousDB := models.DB
				models.DB = db
				defer func() { models.DB = previousDB }()
				var log bytes.Buffer
				logger := logging.Init("local-apply-output", false, false, &log)
				defer logger.Close()
				configuration := &config.Config{}
				if tt.postgresql {
					configuration.PgUser, configuration.PgPassword = "monitor", "test"
				}
				metrics := &models.Metrics{}
				metrics.DB.Conf.Variables = tt.current
				code, status, output := ApplyConfLocal(metrics, &awsApplyRepeater{recommendations: tt.recommendations}, nil, *logger, configuration)
				if code != tt.wantExit || status != tt.wantStatus || output != tt.wantOutput {
					t.Errorf("ApplyConfLocal(%s) = %d/%d %q, want %d/%d %q", tt.name, code, status, output, tt.wantExit, tt.wantStatus, tt.wantOutput)
				}
				if !reflect.DeepEqual(connector.queries, tt.wantQueries) {
					t.Errorf("ApplyConfLocal(%s) queries = %v, want %v", tt.name, connector.queries, tt.wantQueries)
				}
				for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
					if !strings.Contains(log.String(), line) {
						t.Errorf("ApplyConfLocal(%s) agent log missing task output line %q", tt.name, line)
					}
				}
			})
		})
	}
}

type localApplyTestConnector struct {
	queries []string
	err     error
}

func (c *localApplyTestConnector) Connect(context.Context) (driver.Conn, error) {
	return &localApplyTestConn{connector: c}, nil
}

func (c *localApplyTestConnector) Driver() driver.Driver { return localApplyTestDriver{c} }

type localApplyTestDriver struct{ connector *localApplyTestConnector }

func (d localApplyTestDriver) Open(string) (driver.Conn, error) {
	return d.connector.Connect(context.Background())
}

type localApplyTestConn struct{ connector *localApplyTestConnector }

func (*localApplyTestConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("prepare not supported")
}
func (*localApplyTestConn) Close() error { return nil }
func (*localApplyTestConn) Begin() (driver.Tx, error) {
	return nil, errors.New("transactions not supported")
}
func (c *localApplyTestConn) ExecContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Result, error) {
	c.connector.queries = append(c.connector.queries, query)
	return driver.RowsAffected(1), c.connector.err
}
