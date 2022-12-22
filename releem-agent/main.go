// Example of a daemon with echo service
package main

import (
	"flag"
	"io/fs"
	"net"
	"os"

	"github.com/Releem/daemon"
	"github.com/advantageous/go-logback/logging"

	"database/sql"

	"github.com/Releem/mysqlconfigurer/releem-agent/config"
	m "github.com/Releem/mysqlconfigurer/releem-agent/metrics"
	r "github.com/Releem/mysqlconfigurer/releem-agent/repeater"

	_ "github.com/go-sql-driver/mysql"
)

const (
	// name of the service
	name               = "releem-agent"
	description        = "Releem Agent"
	ReleemAgentVersion = "1.0.2"
)

// dependencies that are NOT required by the service, but might be used
var dependencies = []string{"network.target"}

var logger logging.Logger

// Service has embedded daemon
type Service struct {
	daemon.Daemon
}

func IsSocket(path string, logger logging.Logger) bool {
	fileInfo, err := os.Stat(path)
	if err != nil {
		return false
	}
	return fileInfo.Mode().Type() == fs.ModeSocket
}

// Manage by daemon commands or run the daemon
func (service *Service) Manage(logger logging.Logger) (string, error) {

	usage := "Usage: myservice install | remove | start | stop | status"

	// if received any kind of command, do it
	if len(os.Args) > 1 {
		command := os.Args[1]
		switch command {
		case "install":
			return service.Install()
		case "remove":
			return service.Remove()
		case "start":
			return service.Start()
		case "stop":
			return service.Stop()
		case "status":
			return service.Status()
		default:
			return usage, nil
		}
	}
	logger.Println("Starting releem-agent of version is", ReleemAgentVersion)

	// Do something, call your goroutines, etc
	configFile := flag.String("config", "/opt/releem/releem.conf", "Releem config")
	configuration, err := config.LoadConfig(*configFile, logger)
	if err != nil {
		logger.PrintError("Config load failed", err)
	}
	var db *sql.DB
	if IsSocket(configuration.MysqlHost, logger) {
		db, err = sql.Open("mysql", configuration.MysqlUser+":"+configuration.MysqlPassword+"@unix("+configuration.MysqlHost+")/mysql")
	} else if addr := net.ParseIP(configuration.MysqlHost); addr != nil {
		db, err = sql.Open("mysql", configuration.MysqlUser+":"+configuration.MysqlPassword+"@tcp("+configuration.MysqlHost+":"+configuration.MysqlPort+")/mysql")
	}
	if err != nil {
		logger.PrintError("Connection opening to failed", err)
	}
	defer db.Close()

	err = db.Ping()
	if err != nil {
		logger.PrintError("Connection failed", err)
	} else {
		logger.Println("Connect Success to DB", configuration.MysqlHost)
	}

	repeaters := []m.MetricsRepeater{r.NewReleemMetricsRepeater(configuration)}

	gatherers := []m.MetricsGatherer{
		m.NewMysqlStatusMetricsGatherer(nil, db, configuration),
		m.NewMysqlVariablesMetricsGatherer(nil, db, configuration),
		m.NewMysqlLatencyMetricsGatherer(nil, db, configuration)}

	m.RunWorker(gatherers, repeaters, nil, configuration, *configFile, ReleemAgentVersion)

	// never happen, but need to complete code
	return usage, nil
}

func main() {
	logger = logging.NewSimpleLogger("Main")
	srv, err := daemon.New(name, description, daemon.SystemDaemon, dependencies...)
	if err != nil {
		logger.PrintError("Error: ", err)
		os.Exit(1)
	}

	service := &Service{srv}
	status, err := service.Manage(logger)
	if err != nil {
		logger.Println(status, "\nError: ", err)
		os.Exit(1)
	}
	logger.Println(status)
}
