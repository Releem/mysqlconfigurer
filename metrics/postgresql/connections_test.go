package postgresql

import (
	"database/sql"
	"io"
	"testing"

	"github.com/Releem/mysqlconfigurer/config"
	"github.com/Releem/mysqlconfigurer/models"
	"github.com/Releem/mysqlconfigurer/utils"
	logging "github.com/google/logger"
)

func TestAcquireDefaultDatabaseSharesPinnedMainConnection(t *testing.T) {
	recorder := &pgExplainRecordingDriver{}
	sql.Register("releem_pg_default_database_test", recorder)
	main, err := sql.Open("releem_pg_default_database_test", "")
	if err != nil {
		t.Fatal(err)
	}
	pool := utils.NewDatabaseConnectionPool(2)
	defer pool.Close()
	pinned, err := pool.Pin(pgDefaultDatabase, func() (*sql.DB, error) { return main, nil })
	if err != nil {
		t.Fatal(err)
	}

	logger := *logging.Init("pg-default-database-test", false, false, io.Discard)
	// An unreachable host proves the pinned handle is reused instead of reconnecting.
	cfg := &config.Config{PgHost: "127.0.0.1", PgPort: "invalid"}
	for range 2 {
		db, release, err := acquireDefaultDatabase(pool, cfg, logger)
		if err != nil {
			t.Fatal(err)
		}
		release()
		if db != pinned {
			t.Fatal("gatherers must share the pinned main connection")
		}
	}
}

func TestAcquireDefaultDatabaseWithoutPoolUsesModelsDB(t *testing.T) {
	previous := models.DB
	t.Cleanup(func() { models.DB = previous })
	models.DB = &sql.DB{}
	db, release, err := acquireDefaultDatabase(nil, &config.Config{}, *logging.Init("pg-default-database-nil-pool", false, false, io.Discard))
	if err != nil || db != models.DB {
		t.Fatalf("acquireDefaultDatabase(nil) = %p, %v; want models.DB", db, err)
	}
	release()
}
