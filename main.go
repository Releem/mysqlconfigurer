// Example of a daemon with echo service
package main

import (
	"context"
	"flag"
	"io"
	"log"
	"os"
	"runtime"

	"github.com/Releem/daemon"
	"github.com/Releem/mysqlconfigurer/config"
	"github.com/Releem/mysqlconfigurer/metrics"
	"github.com/Releem/mysqlconfigurer/metrics/mysql"
	"github.com/Releem/mysqlconfigurer/metrics/postgresql"
	"github.com/Releem/mysqlconfigurer/metrics/system"
	"github.com/Releem/mysqlconfigurer/models"
	r "github.com/Releem/mysqlconfigurer/repeater"
	"github.com/Releem/mysqlconfigurer/utils"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	_ "github.com/go-sql-driver/mysql"
	logging "github.com/google/logger"

	monitoring "cloud.google.com/go/monitoring/apiv3/v2"
	"google.golang.org/api/sqladmin/v1"
)

const (
	// name of the service
	serviceName        = "releem-agent"
	serviceDescription = "Releem Agent"
)

var logger logging.Logger
var SetConfigRun, GetConfigRun, InitialConfigRun *bool
var ConfigFile, AgentEvent, AgentTask *string

// Service has embedded daemon
type Service struct {
	daemon.Daemon
}
type Programm struct{}

func (programm *Programm) Stop() {
	// Stop should not block. Return with a few seconds.
}

func (programm *Programm) Start() {
	// Start should not block. Do the actual work async.
	go programm.Run()
}

func (programm *Programm) Run() {

	var TypeConfiguration string
	var gatherers, gatherers_metrics, gatherers_configuration, gatherers_query_optimization []models.MetricsGatherer
	var Mode models.ModeType

	if *SetConfigRun {
		TypeConfiguration = "ForceSet"
	} else if *InitialConfigRun {
		TypeConfiguration = "ForceInitial"
	} else if *GetConfigRun {
		TypeConfiguration = "ForceGet"
	} else {
		TypeConfiguration = "Default"
	}

	// Do something, call your goroutines, etc
	logger.Info("Releem-agent version is ", config.ReleemAgentVersion) //
	configuration, err := config.LoadConfig(*ConfigFile, logger)
	if err != nil {
		logger.Error("The agent configuration failed to load", err)
		return
	}
	defer utils.HandlePanic(configuration, logger)

	if configuration.Debug {
		logger.SetLevel(10)
	} else {
		logger.SetLevel(1)
	}

	if len(*AgentEvent) > 0 {
		Mode.Name = "Event"
		Mode.Type = *AgentEvent
	} else if len(*AgentTask) > 0 {
		Mode.Name = "TaskSet"
		Mode.Type = *AgentTask
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
			return
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
			logger.Error(err.Error())
			return
		}

		// Request detailed instance info
		if result != nil && len(result.DBInstances) == 1 {
			configuration.Hostname = configuration.AwsRDSDB
			configuration.MysqlHost = *result.DBInstances[0].Endpoint.Address
			gatherers = append(gatherers, system.NewAWSRDSEnhancedMetricsGatherer(logger, result.DBInstances[0], cwlogsclient, configuration))
			logger.Info("AWS RDS DB instance found: ", configuration.AwsRDSDB)
		} else if result != nil && len(result.DBInstances) > 1 {
			logger.Infof("RDS.DescribeDBInstances: Database has %d instances. Clusters are not supported", len(result.DBInstances))
			return
		} else {
			logger.Info("RDS.DescribeDBInstances: No instances")
			return
		}
	case "gcp/cloudsql":
		logger.Info("InstanceType is gcp/cloudsql")
		logger.Info("Loading GCP configuration")

		// Initialize GCP clients with Application Default Credentials
		ctx := context.Background()

		// Create monitoring client
		monitoringClient, err := monitoring.NewMetricClient(ctx)
		if err != nil {
			logger.Error("Failed to create GCP monitoring client", err)
			return
		}
		defer monitoringClient.Close()

		// Create SQL Admin client
		sqlAdminService, err := sqladmin.NewService(ctx)
		if err != nil {
			logger.Error("Failed to create GCP SQL Admin client", err)
			return
		}
		logger.Info("GSP configuration loaded SUCCESS")
		// Get instance details
		instance, err := sqlAdminService.Instances.Get(configuration.GcpProjectId, configuration.GcpCloudSqlInstance).Do()
		if err != nil {
			logger.Error("Failed to get Cloud SQL instance details", err)
			return
		}

		logger.Info("GCP Cloud SQL instance found: ", instance.Name)

		// Find private IP address for connection
		var connectionIP, typeIP string
		if configuration.GcpCloudSqlPublicConnection {
			typeIP = "PRIMARY"
		} else {
			typeIP = "PRIVATE"
		}
		for _, ipAddr := range instance.IpAddresses {
			if ipAddr.Type == typeIP {
				connectionIP = ipAddr.IpAddress
				break
			}
		}

		if connectionIP != "" {
			// Set connection details
			configuration.Hostname = configuration.GcpCloudSqlInstance
			configuration.MysqlHost = connectionIP
			logger.Info("Using following IP for Cloud SQL connection: ", connectionIP)
		} else {
			logger.Error("No IP addresses found for Cloud SQL instance")
			return
		}

		// Add GCP gatherer
		gatherers = append(gatherers, system.NewGCPCloudSQLEnhancedMetricsGatherer(logger, monitoringClient, sqlAdminService, configuration))

	default:
		logger.Info("InstanceType is Local")
		gatherers = append(gatherers, system.NewOSMetricsGatherer(logger, configuration))

	}

	// Initialize database connection based on database type
	dbType := configuration.GetDatabaseType()
	models.DB = utils.ConnectionDatabase(configuration, logger, "")
	defer models.DB.Close()

	//Init repeaters
	// repeaters := make(map[string]models.MetricsRepeater)
	// repeaters["Metrics"] = models.MetricsRepeater(r.NewReleemConfigurationsRepeater(configuration, models.Mode{Name: "Metrics", Type: ""}))
	// repeaters["Configurations"] = models.MetricsRepeater(r.NewReleemConfigurationsRepeater(configuration, Mode))
	// repeaters["Event"] = models.MetricsRepeater(r.NewReleemConfigurationsRepeater(configuration, Mode))
	// repeaters["TaskGet"] = models.MetricsRepeater(r.NewReleemConfigurationsRepeater(configuration, models.Mode{Name: "TaskGet", Type: ""}))
	// repeaters["TaskStatus"] = models.MetricsRepeater(r.NewReleemConfigurationsRepeater(configuration, models.Mode{Name: "TaskStatus", Type: ""}))
	// repeaters["TaskSet"] = models.MetricsRepeater(r.NewReleemConfigurationsRepeater(configuration, Mode))
	// repeaters["GetConfigurationJson"] = models.MetricsRepeater(r.NewReleemConfigurationsRepeater(configuration, models.Mode{Name: "Configurations", Type: "ForceGetJson"}))
	// repeaters["QueryOptimization"] = models.MetricsRepeater(r.NewReleemConfigurationsRepeater(configuration, models.Mode{Name: "Metrics", Type: "QuerysOptimization"}))
	// repeaters["QueriesOptimization"] = models.MetricsRepeater(r.NewReleemConfigurationsRepeater(configuration, models.Mode{Name: "TaskSet", Type: "queries_optimization"}))
	//var repeaters models.MetricsRepeater
	repeaters := models.MetricsRepeater(r.NewReleemConfigurationsRepeater(configuration, logger))

	//Init gatherers based on database type
	switch dbType {
	case "postgresql":
		gatherers = append(gatherers,
			postgresql.NewPgConfGatherer(logger, configuration),
			postgresql.NewPgInfoBaseGatherer(logger, configuration),
			postgresql.NewPgMetricsBaseGatherer(logger, configuration),
			metrics.NewAgentMetricsGatherer(logger, configuration))
		gatherers_metrics = append(gatherers_metrics, postgresql.NewPgMetricsMetricsBaseGatherer(logger, configuration))
		gatherers_query_optimization = append(gatherers_query_optimization, postgresql.NewPgCollectQueriesOptimization(logger, configuration))
	case "mysql":
		fallthrough
	default:
		gatherers = append(gatherers,
			mysql.NewDbConfGatherer(logger, configuration),
			mysql.NewDbInfoBaseGatherer(logger, configuration),
			mysql.NewDbMetricsBaseGatherer(logger, configuration),
			metrics.NewAgentMetricsGatherer(logger, configuration))
		gatherers_metrics = append(gatherers_metrics, mysql.NewDbMetricsMetricsBaseGatherer(logger, configuration))
		gatherers_configuration = append(gatherers_configuration, mysql.NewDbMetricsGatherer(logger, configuration), mysql.NewDbInfoGatherer(logger, configuration))
		gatherers_query_optimization = append(gatherers_query_optimization, mysql.NewDbCollectQueriesOptimization(logger, configuration))
	}

	metrics.RunWorker(gatherers, gatherers_metrics, gatherers_configuration, gatherers_query_optimization, repeaters, logger, configuration, Mode)

}

// Manage by daemon commands or run the daemon
func (service *Service) Manage(command []string) (string, error) {
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

	return service.Run(&Programm{})

	// never happen, but need to complete code
}
func defaultConfigPath() string {
	switch runtime.GOOS {
	case "windows":
		return "C:\\ProgramData\\ReleemAgent\\releem.conf"
	default: // for Linux and other UNIX-like systems
		return "/opt/releem/releem.conf"
	}
}
func defaultDependencies() []string {
	switch runtime.GOOS {
	case "windows":
		return []string{}
	default: // for Linux and other UNIX-like systems
		return []string{"network.target"}
	}
}

func defaultSystemLogFlag() bool {
	switch runtime.GOOS {
	case "windows":
		return true
	default: // for Linux and other UNIX-like systems
		return false
	}
}

func main() {
	logger = *logging.Init("releem-agent", true, defaultSystemLogFlag(), io.Discard)
	defer logger.Close()
	logging.SetFlags(log.LstdFlags | log.Lshortfile)

	defaultPath := defaultConfigPath()
	SetConfigRun = flag.Bool("f", false, "Run Releem agent to generate configuration")
	GetConfigRun = flag.Bool("c", false, "Run Releem agent to download configuration")
	InitialConfigRun = flag.Bool("initial", false, "Run Releem agent to generate initial configuration")
	ConfigFile = flag.String("config", defaultPath, "Path to the configuration file (default: \""+defaultPath+"\")")
	AgentEvent = flag.String("event", "", "Run Releem agent to handle event")
	AgentTask = flag.String("task", "", "Run Releem agent to execute task")
	flag.Parse()
	command := flag.Args()

	dependencies := defaultDependencies()
	srv, err := daemon.New(serviceName, serviceDescription, daemon.SystemDaemon, dependencies...)
	if err != nil {
		logger.Error("Error: ", err)
		os.Exit(1)
	}
	service := &Service{srv}
	status, err := service.Manage(command)

	if err != nil {
		logger.Info(status, "\nError: ", err)
		os.Exit(1)
	}
	logger.Info(status)
}
