package system

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/Releem/mysqlconfigurer/config"
	"github.com/Releem/mysqlconfigurer/models"
	"github.com/Releem/mysqlconfigurer/utils"
	logging "github.com/google/logger"

	monitoring "cloud.google.com/go/monitoring/apiv3/v2"
	"cloud.google.com/go/monitoring/apiv3/v2/monitoringpb"
	"google.golang.org/api/sqladmin/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type GCPCloudSQLEnhancedMetricsGatherer struct {
	logger           logging.Logger
	debug            bool
	projectID        string
	instanceID       string
	region           string
	monitoringClient *monitoring.MetricClient
	sqlAdminClient   *sqladmin.Service
	configuration    *config.Config
}

type gcpOSMetrics struct {
	Engine         string    `json:"engine"`
	InstanceTier   string    `json:"instanceTier"`
	Timestamp      time.Time `json:"timestamp"`
	Uptime         float64   `json:"uptime"`
	Version        string    `json:"version"`
	State          string    `json:"state"`
	DataDiskSizeGb int64     `json:"dataDiskSizeGb"`
	DataDiskType   string    `json:"dataDiskType"`

	CPU     gcpCPU       `json:"cpu"`
	DiskIO  []gcpDiskIO  `json:"diskIO"`
	FileSys []gcpFileSys `json:"fileSys"`
	Memory  gcpMemory    `json:"memory"`
	Network []gcpNetwork `json:"network"`
	Swap    gcpSwap      `json:"swap"`
}

type gcpCPU struct {
	Utilization float64 `json:"utilization"`
	Usage       float64 `json:"usage"`
	Total       float64 `json:"total"`
}

type gcpDiskIO struct {
	Device          string  `json:"device"`
	ReadIOsPS       float64 `json:"readIOsPS"`
	WriteIOsPS      float64 `json:"writeIOsPS"`
	ReadThroughput  float64 `json:"readThroughput"`
	WriteThroughput float64 `json:"writeThroughput"`
}

type gcpFileSys struct {
	Name        string  `json:"name"`
	MountPoint  string  `json:"mountPoint"`
	Used        int64   `json:"used"`
	Total       int64   `json:"total"`
	UsedPercent float64 `json:"usedPercent"`
}

type gcpMemory struct {
	Total       int64   `json:"total"`
	Used        int64   `json:"used"`
	Utilization float64 `json:"utilization"`
	DbUsed      int64   `json:"dbUsed"`
}

type gcpNetwork struct {
	Interface string  `json:"interface"`
	Rx        float64 `json:"rx"`
	Tx        float64 `json:"tx"`
}

type gcpSwap struct {
	Total float64 `json:"total"`
	Used  float64 `json:"used"`
}

var gcpAvailableMetrics = map[string]string{
	"cpu.reserved_cores":     "cloudsql.googleapis.com/database/cpu/reserved_cores",
	"cpu.utilization":        "cloudsql.googleapis.com/database/cpu/utilization",
	"memory.quota":           "cloudsql.googleapis.com/database/memory/quota",
	"memory.total_usage":     "cloudsql.googleapis.com/database/memory/total_usage",
	"memory.usage":           "cloudsql.googleapis.com/database/memory/usage",
	"memory.utilization":     "cloudsql.googleapis.com/database/memory/utilization",
	"swap.usage":             "cloudsql.googleapis.com/database/swap/bytes_used",
	"disk.read_ops":          "cloudsql.googleapis.com/database/disk/read_ops_count",
	"disk.write_ops":         "cloudsql.googleapis.com/database/disk/write_ops_count",
	"disk.read_bytes":        "cloudsql.googleapis.com/database/disk/read_bytes_count",
	"disk.write_bytes":       "cloudsql.googleapis.com/database/disk/write_bytes_count",
	"disk.bytes_used":        "cloudsql.googleapis.com/database/disk/bytes_used",
	"disk.quota":             "cloudsql.googleapis.com/database/disk/quota",
	"disk.utilization":       "cloudsql.googleapis.com/database/disk/utilization",
	"network.received_bytes": "cloudsql.googleapis.com/database/network/received_bytes_count",
	"network.sent_bytes":     "cloudsql.googleapis.com/database/network/sent_bytes_count",
	"database.uptime":        "cloudsql.googleapis.com/database/uptime",
}

func NewGCPCloudSQLEnhancedMetricsGatherer(logger logging.Logger, monitoringClient *monitoring.MetricClient, sqlAdminClient *sqladmin.Service, configuration *config.Config) *GCPCloudSQLEnhancedMetricsGatherer {
	return &GCPCloudSQLEnhancedMetricsGatherer{
		logger:           logger,
		debug:            configuration.Debug,
		projectID:        configuration.GcpProjectId,
		instanceID:       configuration.GcpCloudSqlInstance,
		region:           configuration.GcpRegion,
		monitoringClient: monitoringClient,
		sqlAdminClient:   sqlAdminClient,
		configuration:    configuration,
	}
}

func (gcpmetrics *GCPCloudSQLEnhancedMetricsGatherer) GetMetrics(metrics *models.Metrics) error {
	defer utils.HandlePanic(gcpmetrics.configuration, gcpmetrics.logger)

	info := make(models.MetricGroupValue)
	metricsMap := make(models.MetricGroupValue)

	// Get instance details
	instance, err := gcpmetrics.sqlAdminClient.Instances.Get(gcpmetrics.projectID, gcpmetrics.instanceID).Do()
	if err != nil {
		gcpmetrics.logger.Errorf("Failed to get Cloud SQL instance details: %s", err)
		return err
	}
	gcpmetrics.logger.V(5).Info("Cloud SQL Instance retrieved successfully")

	ctx := context.Background()

	// Collect metrics from Cloud Monitoring
	gcpMetricsData, err := gcpmetrics.collectRecentMetrics(ctx)
	if err != nil {
		gcpmetrics.logger.Errorf("Failed to collect GCP metrics: %s", err)
		return err
	}
	// Build OS metrics structure
	parsedMetrics := gcpmetrics.buildOSMetrics(instance, gcpMetricsData)

	// Set IOPS
	var readCount, writeCount float64
	for _, diskio := range parsedMetrics.DiskIO {
		readCount += diskio.ReadIOsPS
		writeCount += diskio.WriteIOsPS
	}
	metricsMap["IOP"] = models.MetricGroupValue{"IOPRead": readCount, "IOPWrite": writeCount}

	// Set FileSystem
	metricsMap["FileSystem"] = parsedMetrics.FileSys

	// OS RAM
	metricsMap["PhysicalMemory"] = parsedMetrics.Memory
	info["PhysicalMemory"] = models.MetricGroupValue{"total": parsedMetrics.Memory.Total}
	info["PhysicalMemory"] = utils.MapJoin(info["PhysicalMemory"].(models.MetricGroupValue), models.MetricGroupValue{"swapTotal": parsedMetrics.Swap.Total})

	// Swap
	metricsMap["Swap"] = parsedMetrics.Swap

	// CPU Counts
	info["CPU"] = models.MetricGroupValue{"Counts": parsedMetrics.CPU.Total}

	// DiskIO
	metricsMap["DiskIO"] = parsedMetrics.DiskIO

	// CPU load average
	metricsMap["CPU"] = parsedMetrics.CPU

	info["Host"] = models.MetricGroupValue{
		"InstanceType":   "gcp/cloudsql",
		"Engine":         parsedMetrics.Engine,
		"InstanceTier":   parsedMetrics.InstanceTier,
		"Timestamp":      parsedMetrics.Timestamp,
		"Uptime":         parsedMetrics.Uptime,
		"Version":        parsedMetrics.Version,
		"State":          parsedMetrics.State,
		"DataDiskSizeGb": parsedMetrics.DataDiskSizeGb,
		"DataDiskType":   parsedMetrics.DataDiskType,
	}

	metrics.System.Info = info
	metrics.System.Metrics = metricsMap
	gcpmetrics.logger.V(5).Info("CollectMetrics gcpcloudsqlenhancedmetrics ", metrics.System)

	return nil
}

// Request 1: Latest metrics (separate request for each metric type)
func (gcpmetrics *GCPCloudSQLEnhancedMetricsGatherer) collectRecentMetrics(ctx context.Context) (map[string]float64, error) {
	results := make(map[string]float64)

	// Metrics that need only the latest data point
	recentMetricTypes := make([]string, 0, len(gcpAvailableMetrics))
	for _, metricType := range gcpAvailableMetrics {
		recentMetricTypes = append(recentMetricTypes, metricType)
	}

	// GCP Cloud SQL database_id format is "project_id:instance_id"
	databaseID := fmt.Sprintf("%s:%s", gcpmetrics.projectID, gcpmetrics.instanceID)
	projectName := fmt.Sprintf("projects/%s", gcpmetrics.projectID)

	// Make separate request for each metric type (GCP doesn't support OR on metric.type)
	for _, metricType := range recentMetricTypes {
		filter := fmt.Sprintf(`metric.type="%s" AND resource.label.database_id="%s"`, metricType, databaseID)

		// Use a time window that accounts for metric processing delays
		now := time.Now()
		endTime := now.Add(-1 * time.Second)       // End 30 seconds ago to allow for processing
		startTime := endTime.Add(-5 * time.Minute) // Look back 5 minutes from that point

		req := &monitoringpb.ListTimeSeriesRequest{
			Name:   projectName,
			Filter: filter,
			Interval: &monitoringpb.TimeInterval{
				EndTime:   timestamppb.New(endTime),
				StartTime: timestamppb.New(startTime),
			},
		}

		it := gcpmetrics.monitoringClient.ListTimeSeries(ctx, req)
		ts, err := it.Next()
		if err != nil {
			gcpmetrics.logger.Errorf("No data for metric %s: %v", metricType, err)
			// Log the exact filter being used for debugging
			gcpmetrics.logger.Errorf("Filter used: %s", filter)
			continue // Skip if no data for this metric
		}

		key := gcpmetrics.getKeyFromMetricType(ts.Metric.Type)
		if key != "" && len(ts.Points) > 0 {
			// Get the latest (first) point only
			point := ts.Points[0]
			if point.Value.GetDoubleValue() != 0 {
				results[key] = point.Value.GetDoubleValue()
			} else if point.Value.GetInt64Value() != 0 {
				results[key] = float64(point.Value.GetInt64Value())
			}
		}
	}

	return results, nil
}

// Request 2: CPU Load Averages (different time windows)

func (gcpmetrics *GCPCloudSQLEnhancedMetricsGatherer) getKeyFromMetricType(metricType string) string {
	// Reverse lookup from metric type to our key
	for key, mType := range gcpAvailableMetrics {
		if mType == metricType {
			return key
		}
	}
	return ""
}

func (gcpmetrics *GCPCloudSQLEnhancedMetricsGatherer) buildOSMetrics(instance *sqladmin.DatabaseInstance, metricsData map[string]float64) *gcpOSMetrics {

	// Helper function to get metric value or default
	getMetric := func(key string, defaultValue float64) float64 {
		if val, ok := metricsData[key]; ok && !math.IsNaN(val) && !math.IsInf(val, 0) {
			return val
		}
		return defaultValue
	}

	// Extract engine name from database version
	engine := "MySQL"
	if instance.DatabaseVersion != "" {
		if strings.Contains(strings.ToLower(instance.DatabaseVersion), "postgres") {
			engine = "PostgreSQL"
		}
	}
	// Get instance tier
	var dataDiskSizeGb int64
	var instanceTier, dataDiskType string
	if instance.Settings != nil {
		instanceTier = instance.Settings.Tier
		dataDiskSizeGb = instance.Settings.DataDiskSizeGb
		dataDiskType = instance.Settings.DataDiskType
	}

	return &gcpOSMetrics{
		Engine:         engine,
		InstanceTier:   instanceTier,
		Timestamp:      time.Now(),
		Uptime:         getMetric("database.uptime", 0),
		Version:        instance.DatabaseVersion,
		State:          instance.State,
		DataDiskSizeGb: dataDiskSizeGb,
		DataDiskType:   dataDiskType,

		CPU: gcpCPU{
			Utilization: getMetric("cpu.utilization", 0),
			Usage:       getMetric("cpu.usage_time", 0),
			Total:       getMetric("cpu.reserved_cores", 0),
		},

		DiskIO: []gcpDiskIO{
			{
				Device:          "disk",
				ReadIOsPS:       getMetric("disk.read_ops", 0),
				WriteIOsPS:      getMetric("disk.write_ops", 0),
				ReadThroughput:  getMetric("disk.read_bytes", 0),
				WriteThroughput: getMetric("disk.write_bytes", 0),
			},
		},

		FileSys: []gcpFileSys{
			{
				Name:        "cloudsqlfilesys",
				MountPoint:  "/mysql",
				Used:        int64(getMetric("disk.bytes_used", 0)),
				Total:       int64(getMetric("disk.quota", 0)),
				UsedPercent: float64(getMetric("disk.utilization", 0)),
			},
		},

		Memory: gcpMemory{
			Total:       int64(getMetric("memory.quota", 1073741824)),
			Used:        int64(getMetric("memory.usage", 0)),
			Utilization: float64(getMetric("memory.utilization", 0)),
			DbUsed:      int64(getMetric("memory.total_usage", 0)),
		},

		Network: []gcpNetwork{
			{
				Interface: "eth0",
				Rx:        getMetric("network.received_bytes", 0),
				Tx:        getMetric("network.sent_bytes", 0),
			},
		},

		Swap: gcpSwap{
			Total: getMetric("memory.quota", 1073741824), // Use memory quota as swap total estimate
			Used:  getMetric("swap.usage", 0),
		},
	}
}
