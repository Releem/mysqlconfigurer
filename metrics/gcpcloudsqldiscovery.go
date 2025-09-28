package metrics

import (
	"github.com/Releem/mysqlconfigurer/config"
	"github.com/Releem/mysqlconfigurer/models"
	"github.com/Releem/mysqlconfigurer/utils"
	logging "github.com/google/logger"

	"google.golang.org/api/sqladmin/v1"
)

type GCPCloudSQLInstanceGatherer struct {
	logger         logging.Logger
	debug          bool
	projectID      string
	instanceID     string
	region         string
	sqlAdminClient *sqladmin.Service
	configuration  *config.Config
}

func NewGCPCloudSQLInstanceGatherer(logger logging.Logger, sqlAdminClient *sqladmin.Service, configuration *config.Config) *GCPCloudSQLInstanceGatherer {
	return &GCPCloudSQLInstanceGatherer{
		logger:         logger,
		debug:          configuration.Debug,
		projectID:      configuration.GcpProjectId,
		instanceID:     configuration.GcpCloudSqlInstance,
		region:         configuration.GcpRegion,
		sqlAdminClient: sqlAdminClient,
		configuration:  configuration,
	}
}

func (gcpinstance *GCPCloudSQLInstanceGatherer) GetMetrics(metrics *models.Metrics) error {
	defer utils.HandlePanic(gcpinstance.configuration, gcpinstance.logger)

	info := make(models.MetricGroupValue)

	// Get Cloud SQL instance details
	instance, err := gcpinstance.sqlAdminClient.Instances.Get(gcpinstance.projectID, gcpinstance.instanceID).Do()
	if err != nil {
		gcpinstance.logger.Errorf("Failed to get Cloud SQL instance: %s", err)
		return err
	}

	gcpinstance.logger.Info("Cloud SQL Instance retrieved successfully")
	gcpinstance.logger.V(5).Info("Instance Name: ", instance.Name)
	gcpinstance.logger.V(5).Info("Instance Tier: ", instance.Settings.Tier)
	gcpinstance.logger.V(5).Info("Database Version: ", instance.DatabaseVersion)
	gcpinstance.logger.V(5).Info("Instance State: ", instance.State)

	// Set instance tier as InstanceType for platform processing
	info["Host"] = models.MetricGroupValue{"InstanceType": instance.Settings.Tier}

	metrics.System.Info = info
	gcpinstance.logger.V(5).Info("CollectMetrics gcpcloudsqlinstance", info)

	return nil
}