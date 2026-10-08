package tablesize

import (
	"github.com/Releem/mysqlconfigurer/models"
	"testing"
)

func TestEngineTableCount(t *testing.T) {
	engine := map[string]models.MetricGroupValue{
		"BASE TABLE": {"Table Number": uint64(7)},
		"VIEW":       {"Table Number": uint64(3)},
	}
	if got := EngineTableCount(engine); got != 10 {
		t.Fatalf("count = %d, want 10", got)
	}
	if got := EngineTableCount(nil); got != 0 {
		t.Fatalf("empty count = %d", got)
	}
}
