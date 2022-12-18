// Example of a daemon with echo service
package main

import (
	"database/sql"
	"flag"
	"os"

	"github.com/Releem/daemon"
	"github.com/advantageous/go-logback/logging"

	"github.com/Releem/mysqlconfigurer/releem-agent/config"
	m "github.com/Releem/mysqlconfigurer/releem-agent/metrics"
	r "github.com/Releem/mysqlconfigurer/releem-agent/repeater"

	_ "github.com/go-sql-driver/mysql"
)

const (
	// name of the service
	name        = "releem-agent"
	description = "Releem Agent"
)

// dependencies that are NOT required by the service, but might be used
var dependencies = []string{"network.target"}

var logger logging.Logger

// Service has embedded daemon
type Service struct {
	daemon.Daemon
}

// Manage by daemon commands or run the daemon
func (service *Service) Manage(logger logging.Logger, configFile string, command []string) (string, error) {

	usage := "Usage: myservice install | remove | start | stop | status"

	// if received any kind of command, do it
	if len(command) >= 1 {
		switch command[0] {
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

	// Do something, call your goroutines, etc
	logger.Println("Starting releem-agent of version is", config.ReleemAgentVersion)
	configuration, err := config.LoadConfig(configFile, logger)
	if err != nil {
		logger.PrintError("Config load failed", err)
	}

	db, err := sql.Open("mysql", configuration.MysqlUser+":"+configuration.MysqlPassword+"@tcp("+configuration.MysqlHost+":"+configuration.MysqlPort+")/mysql")
	if err != nil {
		logger.PrintError("Connection opening to failed", err)
	}
	defer db.Close()

	err = db.Ping()
	if err != nil {
		logger.PrintError("Connection failed", err)
		os.Exit(1)
	} else {
		logger.Println("Connect Success to DB", configuration.MysqlHost)
	}
	repeaters := make(map[string][]m.MetricsRepeater)
	repeaters["Metrics"] = []m.MetricsRepeater{r.NewReleemMetricsRepeater(configuration)}
	repeaters["Configurations"] = []m.MetricsRepeater{r.NewReleemConfigurationsRepeater(configuration)}

	gatherers := []m.MetricsGatherer{
		m.NewDbConfGatherer(nil, db, configuration),
		m.NewDbInfoGatherer(nil, db, configuration),
		m.NewDbMetricsGatherer(nil, db, configuration),
		m.NewOSMetricsGatherer(nil, configuration),
		m.NewAgentMetricsGatherer(nil, configuration)}

	// gatherers := []m.MetricsGatherer{
	// 	m.NewOSMetricsGatherer(nil, configuration)}

	m.RunWorker(gatherers, repeaters, nil, configuration, configFile)

	// never happen, but need to complete code
	return usage, nil
}

func main() {
	logger = logging.NewSimpleLogger("Main")

	configFile := flag.String("config", "/opt/releem/releem.conf", "Releem config")
	flag.Parse()
	command := flag.Args()

	srv, err := daemon.New(name, description, daemon.SystemDaemon, dependencies...)
	if err != nil {
		logger.PrintError("Error: ", err)
		os.Exit(1)
	}
	service := &Service{srv}
	status, err := service.Manage(logger, *configFile, command)

	if err != nil {
		logger.Println(status, "\nError: ", err)
		os.Exit(1)
	}
	logger.Println(status)
}
