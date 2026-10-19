package postgresql

import (
	"database/sql/driver"
	"io"
	"strings"
	"testing"

	logging "github.com/google/logger"
	"github.com/lib/pq"
)

func TestExecuteExplainPermissionErrorReturnsGrantHint(t *testing.T) {
	logger := *logging.Init("postgresql-explain-permission-test", false, false, io.Discard)
	defer logger.Close()
	for _, query := range []string{
		"SELECT id FROM stock_quant",
		"SELECT id FROM stock_quant FOR UPDATE",
		"SELECT id FROM stock_quant WHERE id = $1 FOR SHARE",
		"SELECT * FROM stock_report",
		"SELECT substring(name FROM 1 FOR share) FROM stock_quant",
	} {
		t.Run(query, func(t *testing.T) {
			recorder := &pgExplainRecordingDriver{
				explainHandler: func(string) (driver.Rows, error) {
					return nil, &pq.Error{Code: "42501", Message: "permission denied for table stock_quant"}
				},
			}
			_, err := ExecuteExplain(openPgRecordingDB(t, recorder), "42", query, true, logger)
			if err == nil || err.Error() != "need_grant_permission" {
				t.Fatalf("ExecuteExplain(%q) error = %v, want need_grant_permission", query, err)
			}
			for _, executed := range recorder.queries {
				if strings.Contains(executed, "has_table_privilege") {
					t.Errorf("ExecuteExplain(%q) performed an unexpected privilege lookup: %s", query, executed)
				}
			}
		})
	}
}
