// Example of a daemon with echo service
package main

import (
	"flag"
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
	ReleemAgentVersion = "1.0.1"
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
	logger.Println(command,len(command))
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

	logger.Println("Starting releem-agent of version is", ReleemAgentVersion)
	configuration, err := config.LoadConfig(configFile, logger)
	if err != nil {
		logger.PrintError("Config load failed", err)
	}
	time.Sleep(10* time.Second)

	db, err := sql.Open("mysql", configuration.MysqlUser + ":" + configuration.MysqlPassword + "@tcp(" + configuration.MysqlHost + ":" + configuration.MysqlPort + ")/mysql")
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

	repeaters := []m.MetricsRepeater{r.NewReleemMetricsRepeater(configuration)}

	gatherers := []m.MetricsGatherer{
		m.NewMysqlStatusMetricsGatherer(nil, db, configuration),
		m.NewMysqlVariablesMetricsGatherer(nil, db, configuration),
		m.NewMysqlLatencyMetricsGatherer(nil, db, configuration)}

	m.RunWorker(gatherers, repeaters, nil, configuration, configFile, ReleemAgentVersion)

	// never happen, but need to complete code
	return usage, nil
}

func isFlagPassed(name string) bool {
    found := false
    flag.Visit(func(f *flag.Flag) {
        if f.Name == name {
            found = true
        }
    })
    return found
}

func main() {
	logger = logging.NewSimpleLogger("Main")

	daemonMode := flag.Bool("d", false, "Run agent as daemon")
	configFile := flag.String("config", "/opt/releem/releem.conf", "Releem config")
	flag.Parse()
	command := flag.Args()

	logger.Println(*configFile,len(*configFile))

	if *daemonMode {
		var cmd *exec.Cmd
		var updater = &selfupdate.Updater{
			CurrentVersion: ReleemAgentVersion,
			ApiURL:         "http://updates.yourdomain.com/",
			BinURL:         "http://updates.yourdomain.com/",
			DiffURL:        "http://updates.yourdomain.com/",
			Dir:            "update/",
			CmdName:        "myapp", // app name
	  	ForceCheck: true,
		}
		if isFlagPassed("config") {
			if len(command) > 0 {
				cmd = exec.Command(os.Args[0], "--config=" + *configFile, strings.Join(command, " "))
			} else {
				cmd = exec.Command(os.Args[0], "--config=" + *configFile)
			}
		} else {
			if len(command) > 0 {
				cmd = exec.Command(os.Args[0], strings.Join(command, " "))
			} else {
				cmd = exec.Command(os.Args[0])
			}
		}

  	cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Start(); err != nil {
			logger.PrintError("Error: ", err)
		}
		timerUpdate := time.NewTimer(5 * time.Second)
		for {
			select {
			case <-timerUpdate.C:
				logger.Println("Timer update tick")
				timerUpdate.Reset(5 * time.Second)
				if updater != nil {
					updater.BackgroundRun()
					logger.Printf("Next run, I should be %v", updater.Info.Version)

				}
			}
		}
		//cmd.Wait()

	}	else {
		srv, err := daemon.New(name, description, daemon.SystemDaemon, dependencies...)
		if err != nil {
			logger.PrintError("Error: ", err)
			os.Exit(1)
		}
		service := &Service{srv}
		status, err := service.Manage(logger, *configFile, command)
		if err != nil {
			logger.Error(status, "\nError: ", err)
			os.Exit(1)
		}
		logger.Println(status)
	}
}
