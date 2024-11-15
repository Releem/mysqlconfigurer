// Example of a daemon with echo service
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"runtime"

	"github.com/Releem/daemon"
	"github.com/Releem/mysqlconfigurer/config"
	"github.com/Releem/mysqlconfigurer/metrics"
	"github.com/Releem/mysqlconfigurer/models"
	r "github.com/Releem/mysqlconfigurer/repeater"
	"github.com/Releem/mysqlconfigurer/utils"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	"github.com/aws/aws-sdk-go/aws/awserr"
	_ "github.com/go-sql-driver/mysql"
	logging "github.com/google/logger"
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

// func IsSocket(path string, logger logging.Logger) bool {
// 	fileInfo, err := os.Stat(path)
// 	if err != nil {
// 		return false
// 	}
// 	return fileInfo.Mode().Type() == fs.ModeSocket
// }

// Manage by daemon commands or run the daemon
func (service *Service) Manage(logger logging.Logger, configFile string, command []string, TypeConfiguration string, AgentEvent string, AgentTask string) (string, error) {
	var gatherers, gatherers_configuration, gatherers_query_optimization []models.MetricsGatherer
	var Mode models.ModeType
	var configuration *config.Config
	usage := "Usage: myservice install | remove | start | stop | status"

	defer utils.HandlePanic(configuration, logger)

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
	logger.Info("Starting releem-agent of version is ", config.ReleemAgentVersion)
	configuration, err := config.LoadConfig(configFile, logger)
	if err != nil {
		logger.Error("Config load failed", err)
		os.Exit(0)
	}
	if configuration.Debug {
		logger.SetLevel(10)
	} else {
		logger.SetLevel(1)
	}

	if len(AgentEvent) > 0 {
		Mode.Name = "Event"
		Mode.Type = AgentEvent
	} else if len(AgentTask) > 0 {
		Mode.Name = "TaskSet"
		Mode.Type = AgentTask
	} else {
		Mode.Name = "Configurations"
		Mode.Type = TypeConfiguration
	}
	// if Mode.Name != "Event" {
	// Select how we collect instance metrics depending on InstanceType
	switch configuration.InstanceType {
	case "aws/rds":
		logger.Info("InstanceType is aws/rds")
		logger.Info("Loading AWS configuration")

		awscfg, err := awsconfig.LoadDefaultConfig(context.TODO(), awsconfig.WithRegion(configuration.AwsRegion))
		if err != nil {
			logger.Error("Load AWS configuration FAILED", err)
			return "Error", err
		} else {
			logger.Info("AWS configuration loaded SUCCESS")
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
				return "Error", aerr
			} else {
				// Print the error, cast err to awserr.Error to get the Code and
				// Message from an error.
				logger.Error(err.Error())
				return "Error", err
			}
		}

		logger.Info("RDS.DescribeDBInstances SUCCESS")

		// Request detailed instance info
		if result != nil && len(result.DBInstances) == 1 {
			//	gatherers = append(gatherers, models.NewAWSRDSMetricsGatherer(logger, cwclient, configuration))
			//	gatherers = append(gatherers, models.NewAWSRDSInstanceGatherer(logger, rdsclient, ec2client, configuration))
			configuration.Hostname = configuration.AwsRDSDB
			configuration.MysqlHost = *result.DBInstances[0].Endpoint.Address
			gatherers = append(gatherers, metrics.NewAWSRDSEnhancedMetricsGatherer(logger, result.DBInstances[0], cwlogsclient, configuration))
		} else if result != nil && len(result.DBInstances) > 1 {
			logger.Infof("RDS.DescribeDBInstances: Database has %d instances. Clusters are not supported", len(result.DBInstances))
			return "Error", fmt.Errorf("RDS.DescribeDBInstances: Database has %d instances. Clusters are not supported", len(result.DBInstances))
		} else {
			logger.Info("RDS.DescribeDBInstances: No instances")
			return "Error", fmt.Errorf("RDS.DescribeDBInstances: No instances")
		}
	default:
		logger.Info("InstanceType is Local")
		gatherers = append(gatherers, metrics.NewOSMetricsGatherer(logger, configuration))

	}

	models.DB = utils.ConnectionDatabase(configuration, logger, "mysql")
	defer models.DB.Close()

	//Init repeaters
	// repeaters := make(map[string]models.MetricsRepeater)
	// repeaters["Metrics"] = models.MetricsRepeater(r.NewReleemConfigurationsRepeater(configuration, models.Mode{Name: "Metrics", Type: ""}))
	// repeaters["Configurations"] = models.MetricsRepeater(r.NewReleemConfigurationsRepeater(configuration, Mode))
	// repeaters["Event"] = models.MetricsRepeater(r.NewReleemConfigurationsRepeater(configuration, Mode))
	// repeaters["TaskGet"] = models.MetricsRepeater(r.NewReleemConfigurationsRepeater(configuration, models.Mode{Name: "TaskGet", Type: ""}))
	// repeaters["TaskStatus"] = models.MetricsRepeater(r.NewReleemConfigurationsRepeater(configuration, models.Mode{Name: "TaskStatus", Type: ""}))
	// repeaters["TaskSet"] = models.MetricsRepeater(r.NewReleemConfigurationsRepeater(configuration, Mode))
	// repeaters["GetConfigurationJson"] = models.MetricsRepeater(r.NewReleemConfigurationsRepeater(configuration, models.Mode{Name: "Configurations", Type: "get-json"}))
	// repeaters["QueryOptimization"] = models.MetricsRepeater(r.NewReleemConfigurationsRepeater(configuration, models.Mode{Name: "Metrics", Type: "QuerysOptimization"}))
	// repeaters["QueriesOptimization"] = models.MetricsRepeater(r.NewReleemConfigurationsRepeater(configuration, models.Mode{Name: "TaskSet", Type: "queries_optimization"}))
	//var repeaters models.MetricsRepeater
	repeaters := models.MetricsRepeater(r.NewReleemConfigurationsRepeater(configuration, logger))

	//Init gatherers
	gatherers = append(gatherers,
		metrics.NewDbConfGatherer(logger, configuration),
		metrics.NewDbInfoGatherer(logger, configuration),
		metrics.NewDbMetricsBaseGatherer(logger, configuration),
		metrics.NewAgentMetricsGatherer(logger, configuration))
	gatherers_configuration = append(gatherers_configuration, metrics.NewDbMetricsGatherer(logger, configuration))
	gatherers_query_optimization = append(gatherers_query_optimization, metrics.NewDbCollectQueriesOptimization(logger, configuration))

	metrics.RunWorker(gatherers, gatherers_configuration, gatherers_query_optimization, repeaters, logger, configuration, configFile, Mode)

	// never happen, but need to complete code
	return usage, nil
}
func defaultConfigPath() string {
	switch runtime.GOOS {
	case "windows":
		return "C:\\ProgramData\\ReleemAgent\\releem.conf"
	default: // для Linux и других UNIX-подобных систем
		return "/opt/releem/releem.conf"
	}
}
func main() {
	var TypeConfiguration string
	logger = *logging.Init("releem-agent", true, true, io.Discard)
	defer logger.Close()
	logging.SetFlags(log.LstdFlags | log.Lshortfile)

	defaultPath := defaultConfigPath()
	configFile := flag.String("config", defaultPath, "Releem agent config")
	SetConfigRun := flag.Bool("f", false, "Releem agent generate config")
	GetConfigRun := flag.Bool("c", false, "Releem agent get config")

	AgentEvent := flag.String("event", "", "Releem agent type event")
	AgentTask := flag.String("task", "", "Releem agent task name")

	flag.Parse()
	command := flag.Args()
	if *SetConfigRun {
		TypeConfiguration = "set"
	} else if *GetConfigRun {
		TypeConfiguration = "get"
	} else {
		TypeConfiguration = "default"
	}

	srv, err := daemon.New(name, description, daemon.SystemDaemon, dependencies...)
	if err != nil {
		logger.Error("Error: ", err)
		os.Exit(1)
	}
	service := &Service{srv}
	status, err := service.Manage(logger, *configFile, command, TypeConfiguration, *AgentEvent, *AgentTask)

	if err != nil {
		logger.Info(status, "\nError: ", err)
		os.Exit(1)
	}
	logger.Info(status)
}
