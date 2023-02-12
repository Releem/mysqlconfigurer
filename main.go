// Example of a daemon with echo service
package main

import (
	"context"
	"database/sql"
	"flag"
	"io/fs"
	"os"

	"github.com/Releem/daemon"
	"github.com/Releem/mysqlconfigurer/config"
	m "github.com/Releem/mysqlconfigurer/metrics"
	r "github.com/Releem/mysqlconfigurer/repeater"
	"github.com/advantageous/go-logback/logging"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	"github.com/aws/aws-sdk-go/aws/awserr"
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

func IsSocket(path string, logger logging.Logger) bool {
	fileInfo, err := os.Stat(path)
	if err != nil {
		return false
	}
	return fileInfo.Mode().Type() == fs.ModeSocket
}

// Manage by daemon commands or run the daemon
func (service *Service) Manage(logger logging.Logger, configFile string, command []string, FirstRun bool, AgentEvents string) (string, error) {
	var gatherers []m.MetricsGatherer
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

	if len(AgentEvents) > 0 {
		configuration.Mode.Name = "Events"
		configuration.Mode.ModeType = AgentEvents
	} else {
		configuration.Mode.Name = "Configurations"
	}
	if FirstRun {
		configuration.Mode.ModeType = "FirstRun"
	}
	if configuration.Mode.Name != "Events" {
		// Select how we collect instance metrics depending on InstanceType
		switch configuration.InstanceType {
		case "aws/rds":
			logger.Println("InstanceType is aws/rds")
			logger.Println("Loading AWS configuration")

			awscfg, err := awsconfig.LoadDefaultConfig(context.TODO(), awsconfig.WithRegion(configuration.AwsRegion))
			if err != nil {
				logger.PrintError("Load AWS configuration FAILED", err)
			} else {
				logger.Println("AWS configuration loaded SUCCESS")
			}

			cwlogsclient := cloudwatchlogs.NewFromConfig(awscfg)
			//	cwclient := cloudwatch.NewFromConfig(awscfg)
			rdsclient := rds.NewFromConfig(awscfg)
			//	ec2client := ec2.NewFromConfig(awscfg)

			// Prepare request to RDS
			input := &rds.DescribeDBInstancesInput{
				DBInstanceIdentifier: &configuration.AwsRDSDB,
			}

			// Request to RDS
			result, err := rdsclient.DescribeDBInstances(context.TODO(), input)

			if err != nil {
				if aerr, ok := err.(awserr.Error); ok {
					logger.Error(aerr.Error())

				} else {
					// Print the error, cast err to awserr.Error to get the Code and
					// Message from an error.
					logger.Error(err.Error())
				}
			}

			logger.Debug("RDS.DescribeDBInstances SUCCESS")

			// Request detailed instance info
			if len(result.DBInstances) == 1 {
				//	gatherers = append(gatherers, m.NewAWSRDSMetricsGatherer(nil, cwclient, configuration))
				//	gatherers = append(gatherers, m.NewAWSRDSInstanceGatherer(nil, rdsclient, ec2client, configuration))
				configuration.Hostname = configuration.AwsRDSDB
				configuration.MysqlHost = *result.DBInstances[0].Endpoint.Address
				gatherers = append(gatherers, m.NewAWSRDSEnhancedMetricsGatherer(nil, result.DBInstances[0], cwlogsclient, configuration))
			} else if len(result.DBInstances) > 1 {
				logger.Println("RDS.DescribeDBInstances: Database has %d instances. Clusters are not supported", len(result.DBInstances))
			} else {
				logger.Println("RDS.DescribeDBInstances: No instances")
			}
		default:
			logger.Println("InstanceType is Local")
			gatherers = append(gatherers, m.NewOSMetricsGatherer(nil, configuration))

		}
	}
	// Init connection DB
	var db *sql.DB
	if IsSocket(configuration.MysqlHost, logger) {
		db, err = sql.Open("mysql", configuration.MysqlUser+":"+configuration.MysqlPassword+"@unix("+configuration.MysqlHost+")/mysql")
	} else {
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
	//Init repeaters
	repeaters := make(map[string][]m.MetricsRepeater)
	repeaters["Metrics"] = []m.MetricsRepeater{r.NewReleemMetricsRepeater(configuration)}
	repeaters["Configurations"] = []m.MetricsRepeater{r.NewReleemConfigurationsRepeater(configuration)}
	repeaters["Events"] = []m.MetricsRepeater{r.NewReleemEventsRepeater(configuration)}

	//Init gatherers
	if configuration.Mode.Name != "Events" {
		gatherers = append(gatherers,
			m.NewDbConfGatherer(nil, db, configuration),
			m.NewDbInfoGatherer(nil, db, configuration),
			m.NewDbMetricsGatherer(nil, db, configuration),
			m.NewAgentMetricsGatherer(nil, configuration))
	} else {
		gatherers = append(gatherers,
			m.NewDbConfGatherer(nil, db, configuration),
			m.NewAgentMetricsGatherer(nil, configuration))
	}

	m.RunWorker(gatherers, repeaters, nil, configuration, configFile)

	// never happen, but need to complete code
	return usage, nil
}

func main() {
	logger = logging.NewSimpleLogger("Main")

	configFile := flag.String("config", "/opt/releem/releem.conf", "Releem agent config")
	FirstRun := flag.Bool("f", false, "Releem agent generate config")
	AgentEvents := flag.String("event", "", "Releem agent type event")

	flag.Parse()
	command := flag.Args()

	srv, err := daemon.New(name, description, daemon.SystemDaemon, dependencies...)
	if err != nil {
		logger.PrintError("Error: ", err)
		os.Exit(1)
	}
	service := &Service{srv}
	status, err := service.Manage(logger, *configFile, command, *FirstRun, *AgentEvents)

	if err != nil {
		logger.Println(status, "\nError: ", err)
		os.Exit(1)
	}
	logger.Println(status)
}
