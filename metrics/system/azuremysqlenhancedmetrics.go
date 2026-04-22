package system

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/monitor/armmonitor"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/mysql/armmysqlflexibleservers"
	"github.com/Releem/mysqlconfigurer/config"
	"github.com/Releem/mysqlconfigurer/models"
	"github.com/Releem/mysqlconfigurer/utils"
	logging "github.com/google/logger"
)

type AzureMySQLEnhancedMetricsGatherer struct {
	logger        logging.Logger
	debug         bool
	subscription  string
	resourceGroup string
	serverName    string
	credential    *azidentity.DefaultAzureCredential
	configuration *config.Config
}

type azureOSMetrics struct {
	Engine         string    `json:"engine"`
	InstanceTier   string    `json:"instanceTier"`
	Timestamp      time.Time `json:"timestamp"`
	Uptime         float64   `json:"uptime"`
	Version        string    `json:"version"`
	State          string    `json:"state"`
	Location       string    `json:"location"`
	DataDiskSizeGb int64     `json:"dataDiskSizeGb"`

	CPU     azureCPU       `json:"cpu"`
	DiskIO  []azureDiskIO  `json:"diskIO"`
	FileSys []azureFileSys `json:"fileSys"`
	Memory  azureMemory    `json:"memory"`
	Network []azureNetwork `json:"network"`
	Swap    azureSwap      `json:"swap"`
}

type azureCPU struct {
	Utilization float64 `json:"utilization"`
	Total       float64 `json:"total"`
}

type azureDiskIO struct {
	Device          string  `json:"device"`
	ReadIOsPS       float64 `json:"readIOsPS"`
	WriteIOsPS      float64 `json:"writeIOsPS"`
	Utilization     float64 `json:"utilization"`
	ReadThroughput  float64 `json:"readThroughput"`
	WriteThroughput float64 `json:"writeThroughput"`
}

type azureFileSys struct {
	Name        string  `json:"name"`
	MountPoint  string  `json:"mountPoint"`
	Used        int64   `json:"used"`
	Total       int64   `json:"total"`
	UsedPercent float64 `json:"usedPercent"`
}

type azureMemory struct {
	Total       int64   `json:"total"`
	Used        int64   `json:"used"`
	Utilization float64 `json:"utilization"`
}

type azureNetwork struct {
	Interface string  `json:"interface"`
	Rx        float64 `json:"rx"`
	Tx        float64 `json:"tx"`
}

type azureSwap struct {
	Total float64 `json:"total"`
	Used  float64 `json:"used"`
}

var azureAvailableMetrics = map[string]string{
	"cpu.utilization":        "cpu_percent",
	"memory.utilization":     "memory_percent",
	"disk.bytes_used":        "storage_used",
	"disk.quota":             "storage_limit",
	"disk.utilization":       "storage_percent",
	"disk.io_count":          "storage_io_count",
	"disk.io_utilization":    "io_consumption_percent",
	"network.received_bytes": "network_bytes_ingress",
	"network.sent_bytes":     "network_bytes_egress",
	"database.uptime":        "Uptime",
}

func NewAzureMySQLEnhancedMetricsGatherer(logger logging.Logger, credential *azidentity.DefaultAzureCredential, configuration *config.Config) *AzureMySQLEnhancedMetricsGatherer {
	return &AzureMySQLEnhancedMetricsGatherer{
		logger:        logger,
		debug:         configuration.Debug,
		subscription:  configuration.AzureSubscriptionID,
		resourceGroup: configuration.AzureResourceGroup,
		serverName:    configuration.AzureMySQLServer,
		credential:    credential,
		configuration: configuration,
	}
}

func (azuremetrics *AzureMySQLEnhancedMetricsGatherer) GetMetrics(metrics *models.Metrics) error {
	defer utils.HandlePanic(azuremetrics.configuration, azuremetrics.logger)

	info := make(models.MetricGroupValue)
	metricsMap := make(models.MetricGroupValue)

	ctx := context.Background()
	instance, err := azuremetrics.GetServer(ctx)
	if err != nil {
		azuremetrics.logger.Errorf("Failed to get Azure Database for MySQL server details: %s", err)
		return err
	}
	azuremetrics.logger.V(5).Info("Azure Database for MySQL server retrieved successfully")

	resourceID := azuremetrics.serverResourceID(instance)
	metricsData, err := azuremetrics.collectRecentMetrics(ctx, resourceID)
	if err != nil {
		azuremetrics.logger.Errorf("Failed to collect Azure metrics: %s", err)
		return err
	}

	parsedMetrics := azuremetrics.buildOSMetrics(instance, metricsData)

	var readCount, writeCount float64
	for _, diskio := range parsedMetrics.DiskIO {
		readCount += diskio.ReadIOsPS
		writeCount += diskio.WriteIOsPS
	}
	metricsMap["IOP"] = models.MetricGroupValue{"IOPRead": readCount, "IOPWrite": writeCount}

	metricsMap["FileSystem"] = parsedMetrics.FileSys

	metricsMap["PhysicalMemory"] = parsedMetrics.Memory

	metricsMap["Swap"] = parsedMetrics.Swap
	metricsMap["DiskIO"] = parsedMetrics.DiskIO
	metricsMap["CPU"] = parsedMetrics.CPU

	info["Host"] = models.MetricGroupValue{
		"InstanceType":    "azure/mysql",
		"platform":        "azure",
		"platformVersion": strings.TrimSpace(fmt.Sprintf("mysql flexible server %s", parsedMetrics.Version)),
		"Engine":          parsedMetrics.Engine,
		"InstanceTier":    parsedMetrics.InstanceTier,
		"Timestamp":       parsedMetrics.Timestamp,
		"Uptime":          parsedMetrics.Uptime,
		"Version":         parsedMetrics.Version,
		"State":           parsedMetrics.State,
		"Location":        parsedMetrics.Location,
		"DataDiskSizeGb":  parsedMetrics.DataDiskSizeGb,
	}

	metrics.System.Info = info
	metrics.System.Metrics = metricsMap
	azuremetrics.logger.V(5).Info("CollectMetrics azuremysqlenhancedmetrics ", metrics.System)

	return nil
}

func (azuremetrics *AzureMySQLEnhancedMetricsGatherer) GetServer(ctx context.Context) (*armmysqlflexibleservers.Server, error) {
	subscriptionID, resourceGroupName, serverName := azuremetrics.getServerIdentifiers()
	if subscriptionID == "" || resourceGroupName == "" || serverName == "" {
		return nil, fmt.Errorf("azure_subscription_id, azure_resource_group and azure_mysql_server must be set")
	}

	client, err := armmysqlflexibleservers.NewServersClient(subscriptionID, azuremetrics.credential, nil)
	if err != nil {
		return nil, err
	}

	response, err := client.Get(ctx, resourceGroupName, serverName, nil)
	if err != nil {
		return nil, err
	}

	return &response.Server, nil
}

func (azuremetrics *AzureMySQLEnhancedMetricsGatherer) collectRecentMetrics(ctx context.Context, resourceID string) (map[string]float64, error) {
	if resourceID == "" {
		return nil, fmt.Errorf("azure mysql resource id is empty")
	}

	metricNames := make([]string, 0, len(azureAvailableMetrics))
	for _, metricName := range azureAvailableMetrics {
		metricNames = append(metricNames, metricName)
	}

	endTime := time.Now().UTC().Add(-1 * time.Minute)
	startTime := endTime.Add(-10 * time.Minute)

	if azuremetrics.subscription == "" {
		return nil, fmt.Errorf("azure subscription id is required to collect Azure Monitor metrics")
	}

	client, err := armmonitor.NewMetricsClient(azuremetrics.subscription, azuremetrics.credential, nil)
	if err != nil {
		return nil, err
	}

	aggregation := "Average,Total,Maximum"
	interval := "PT1M"
	metricNamesParam := strings.Join(metricNames, ",")
	timespan := fmt.Sprintf("%s/%s", startTime.Format(time.RFC3339), endTime.Format(time.RFC3339))

	response, err := client.List(ctx, resourceID, &armmonitor.MetricsClientListOptions{
		Aggregation: &aggregation,
		Interval:    &interval,
		Metricnames: &metricNamesParam,
		Timespan:    &timespan,
	})
	if err != nil {
		return nil, err
	}

	results := make(map[string]float64)
	for _, metric := range response.Value {
		if metric == nil || metric.Name == nil || metric.Name.Value == nil {
			continue
		}
		key := getAzureKeyFromMetricName(*metric.Name.Value)
		if key == "" {
			continue
		}
		if value, ok := latestAzureMetricValue(metric); ok {
			results[key] = value
		}
	}

	return results, nil
}

func (azuremetrics *AzureMySQLEnhancedMetricsGatherer) getServerIdentifiers() (string, string, string) {
	return azuremetrics.subscription, azuremetrics.resourceGroup, azuremetrics.serverName
}

func (azuremetrics *AzureMySQLEnhancedMetricsGatherer) serverResourceID(instance *armmysqlflexibleservers.Server) string {
	if instance != nil && instance.ID != nil && *instance.ID != "" {
		return *instance.ID
	}
	subscriptionID, resourceGroupName, serverName := azuremetrics.getServerIdentifiers()
	if subscriptionID == "" || resourceGroupName == "" || serverName == "" {
		return ""
	}
	return fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.DBforMySQL/flexibleServers/%s",
		subscriptionID,
		resourceGroupName,
		serverName)
}

func getAzureKeyFromMetricName(metricName string) string {
	for key, mName := range azureAvailableMetrics {
		if mName == metricName {
			return key
		}
	}
	return ""
}

func latestAzureMetricValue(metric *armmonitor.Metric) (float64, bool) {
	for _, timeseries := range metric.Timeseries {
		if timeseries == nil {
			continue
		}
		for i := len(timeseries.Data) - 1; i >= 0; i-- {
			point := timeseries.Data[i]
			if point == nil {
				continue
			}
			if point.Average != nil {
				return *point.Average, true
			}
			if point.Total != nil {
				return *point.Total, true
			}
			if point.Maximum != nil {
				return *point.Maximum, true
			}
			if point.Minimum != nil {
				return *point.Minimum, true
			}
		}
	}
	return 0, false
}

func (azuremetrics *AzureMySQLEnhancedMetricsGatherer) buildOSMetrics(instance *armmysqlflexibleservers.Server, metricsData map[string]float64) *azureOSMetrics {
	getMetric := func(key string, defaultValue float64) float64 {
		if val, ok := metricsData[key]; ok && !math.IsNaN(val) && !math.IsInf(val, 0) {
			return val
		}
		return defaultValue
	}

	storageLimit := int64(getMetric("disk.quota", 0))
	storageSizeGB := int64(0)
	if instance != nil && instance.Properties != nil && instance.Properties.Storage != nil && instance.Properties.Storage.StorageSizeGB != nil {
		storageSizeGB = int64(*instance.Properties.Storage.StorageSizeGB)
	}
	if storageLimit == 0 && storageSizeGB > 0 {
		storageLimit = storageSizeGB * 1024 * 1024 * 1024
	}
	// Extract engine name from database version
	engine := "MySQL"

	skuName, skuTier := azureServerSKU(instance)

	return &azureOSMetrics{
		Engine:         engine,
		InstanceTier:   strings.TrimSpace(skuTier + " " + skuName),
		Timestamp:      time.Now(),
		Uptime:         getMetric("database.uptime", 0),
		Version:        azureServerVersion(instance),
		State:          azureServerState(instance),
		Location:       azureServerLocation(instance),
		DataDiskSizeGb: storageSizeGB,

		CPU: azureCPU{
			Utilization: getMetric("cpu.utilization", 0),
		},

		DiskIO: []azureDiskIO{
			{
				Device:          "disk",
				ReadIOsPS:       getMetric("disk.io_count", 0),
				WriteIOsPS:      0,
				Utilization:     getMetric("disk.io_utilization", 0),
				ReadThroughput:  0,
				WriteThroughput: 0,
			},
		},

		FileSys: []azureFileSys{
			{
				Name:        "azuremysqlfilesys",
				MountPoint:  "/app",
				Used:        int64(getMetric("disk.bytes_used", 0)),
				Total:       storageLimit,
				UsedPercent: getMetric("disk.utilization", 0),
			},
		},

		Memory: azureMemory{
			Utilization: getMetric("memory.utilization", 0),
		},

		Network: []azureNetwork{
			{
				Interface: "eth0",
				Rx:        getMetric("network.received_bytes", 0),
				Tx:        getMetric("network.sent_bytes", 0),
			},
		},
	}
}

func azureServerSKU(instance *armmysqlflexibleservers.Server) (string, string) {
	if instance == nil || instance.SKU == nil {
		return "", ""
	}
	var name, tier string
	if instance.SKU.Name != nil {
		name = *instance.SKU.Name
	}
	if instance.SKU.Tier != nil {
		tier = string(*instance.SKU.Tier)
	}
	return name, tier
}

func azureServerVersion(instance *armmysqlflexibleservers.Server) string {
	if instance == nil || instance.Properties == nil || instance.Properties.Version == nil {
		return ""
	}
	return string(*instance.Properties.Version)
}

func azureServerState(instance *armmysqlflexibleservers.Server) string {
	if instance == nil || instance.Properties == nil || instance.Properties.State == nil {
		return ""
	}
	return string(*instance.Properties.State)
}

func azureServerLocation(instance *armmysqlflexibleservers.Server) string {
	if instance == nil || instance.Location == nil {
		return ""
	}
	return *instance.Location
}
