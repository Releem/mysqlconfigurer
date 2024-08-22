// Example of a daemon with echo service
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/Releem/daemon"
	"github.com/Releem/mysqlconfigurer/config"
	m "github.com/Releem/mysqlconfigurer/metrics"
	r "github.com/Releem/mysqlconfigurer/repeater"
	u "github.com/Releem/mysqlconfigurer/utils"

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

// func IsSocket(path string, logger logging.Logger) bool {
// 	fileInfo, err := os.Stat(path)
// 	if err != nil {
// 		return false
// 	}
// 	return fileInfo.Mode().Type() == fs.ModeSocket
// }

// Manage by daemon commands or run the daemon
func (service *Service) Manage(logger logging.Logger, configFile string, command []string, TypeConfiguration string, AgentEvent string, AgentTask string) (string, error) {
	var gatherers, gatherers_configuration, gatherers_query_optimization []m.MetricsGatherer
	var Mode m.ModeT
	var configuration *config.Config
	usage := "Usage: myservice install | remove | start | stop | status"

	defer m.HandlePanic(configuration, logger)

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
		os.Exit(0)
	}

	if len(AgentEvent) > 0 {
		Mode.Name = "Event"
		Mode.ModeType = AgentEvent
	} else if len(AgentTask) > 0 {
		Mode.Name = "TaskSet"
		Mode.ModeType = AgentTask
	} else {
		Mode.Name = "Configurations"
		Mode.ModeType = TypeConfiguration
	}
	// if Mode.Name != "Event" {
	// Select how we collect instance metrics depending on InstanceType
	switch configuration.InstanceType {
	case "aws/rds":
		logger.Println("InstanceType is aws/rds")
		logger.Println("Loading AWS configuration")

		awscfg, err := awsconfig.LoadDefaultConfig(context.TODO(), awsconfig.WithRegion(configuration.AwsRegion))
		if err != nil {
			logger.PrintError("Load AWS configuration FAILED", err)
			return "Error", err
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
				return "Error", aerr
			} else {
				// Print the error, cast err to awserr.Error to get the Code and
				// Message from an error.
				logger.Error(err.Error())
				return "Error", err
			}
		}

		logger.Debug("RDS.DescribeDBInstances SUCCESS")

		// Request detailed instance info
		if result != nil && len(result.DBInstances) == 1 {
			//	gatherers = append(gatherers, m.NewAWSRDSMetricsGatherer(nil, cwclient, configuration))
			//	gatherers = append(gatherers, m.NewAWSRDSInstanceGatherer(nil, rdsclient, ec2client, configuration))
			configuration.Hostname = configuration.AwsRDSDB
			configuration.MysqlHost = *result.DBInstances[0].Endpoint.Address
			gatherers = append(gatherers, m.NewAWSRDSEnhancedMetricsGatherer(nil, result.DBInstances[0], cwlogsclient, configuration))
		} else if result != nil && len(result.DBInstances) > 1 {
			logger.Println("RDS.DescribeDBInstances: Database has %d instances. Clusters are not supported", len(result.DBInstances))
			return "Error", fmt.Errorf("RDS.DescribeDBInstances: Database has %d instances. Clusters are not supported", len(result.DBInstances))
		} else {
			logger.Println("RDS.DescribeDBInstances: No instances")
			return "Error", fmt.Errorf("RDS.DescribeDBInstances: No instances")
		}
	default:
		logger.Println("InstanceType is Local")
		gatherers = append(gatherers, m.NewOSMetricsGatherer(nil, configuration))

	}

	config.DB = u.ConnectionDatabase(configuration, logger, "mysql")
	defer config.DB.Close()

	//Init repeaters
	// repeaters := make(map[string]m.MetricsRepeater)
	// repeaters["Metrics"] = m.MetricsRepeater(r.NewReleemConfigurationsRepeater(configuration, m.Mode{Name: "Metrics", ModeType: ""}))
	// repeaters["Configurations"] = m.MetricsRepeater(r.NewReleemConfigurationsRepeater(configuration, Mode))
	// repeaters["Event"] = m.MetricsRepeater(r.NewReleemConfigurationsRepeater(configuration, Mode))
	// repeaters["TaskGet"] = m.MetricsRepeater(r.NewReleemConfigurationsRepeater(configuration, m.Mode{Name: "TaskGet", ModeType: ""}))
	// repeaters["TaskStatus"] = m.MetricsRepeater(r.NewReleemConfigurationsRepeater(configuration, m.Mode{Name: "TaskStatus", ModeType: ""}))
	// repeaters["TaskSet"] = m.MetricsRepeater(r.NewReleemConfigurationsRepeater(configuration, Mode))
	// repeaters["GetConfigurationJson"] = m.MetricsRepeater(r.NewReleemConfigurationsRepeater(configuration, m.Mode{Name: "Configurations", ModeType: "get-json"}))
	// repeaters["QueryOptimization"] = m.MetricsRepeater(r.NewReleemConfigurationsRepeater(configuration, m.Mode{Name: "Metrics", ModeType: "QuerysOptimization"}))
	// repeaters["QueriesOptimization"] = m.MetricsRepeater(r.NewReleemConfigurationsRepeater(configuration, m.Mode{Name: "TaskSet", ModeType: "queries_optimization"}))
	//var repeaters m.MetricsRepeater
	repeaters := m.MetricsRepeater(r.NewReleemConfigurationsRepeater(configuration))

	//Init gatherers
	gatherers = append(gatherers,
		m.NewDbConfGatherer(nil, configuration),
		m.NewDbInfoGatherer(nil, configuration),
		m.NewDbMetricsBaseGatherer(nil, configuration),
		m.NewAgentMetricsGatherer(nil, configuration))
	gatherers_configuration = append(gatherers_configuration, m.NewDbMetricsGatherer(nil, configuration))
	gatherers_query_optimization = append(gatherers_query_optimization, m.NewDbCollectQueriesOptimization(nil, configuration))

	m.RunWorker(gatherers, gatherers_configuration, gatherers_query_optimization, repeaters, nil, configuration, configFile, Mode)

	// never happen, but need to complete code
	return usage, nil
}

func main() {
	var TypeConfiguration string
	logger = logging.NewSimpleLogger("Main")

	configFile := flag.String("config", "/opt/releem/releem.conf", "Releem agent config")
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
		logger.PrintError("Error: ", err)
		os.Exit(1)
	}
	service := &Service{srv}
	status, err := service.Manage(logger, *configFile, command, TypeConfiguration, *AgentEvent, *AgentTask)

	if err != nil {
		logger.Println(status, "\nError: ", err)
		os.Exit(1)
	}
	logger.Println(status)
}
