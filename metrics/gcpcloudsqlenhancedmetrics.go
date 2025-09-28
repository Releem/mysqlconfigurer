package metrics

import (
	"context"
	"encoding/json"
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
	Engine             string    `json:"engine"`
	InstanceID         string    `json:"instanceID"`
	InstanceResourceID string    `json:"instanceResourceID"`
	InstanceTier       string    `json:"instanceTier"`
	NumVCPUs           int       `json:"numVCPUs"`
	Timestamp          time.Time `json:"timestamp"`
	Uptime             string    `json:"uptime"`
	Version            float64   `json:"version"`

	CPUUtilization    gcpCPUUtilization    `json:"cpuUtilization"`
	DiskIO            []gcpDiskIO          `json:"diskIO"`
	FileSys           []gcpFileSys         `json:"fileSys"`
	LoadAverageMinute gcpLoadAverageMinute `json:"loadAverageMinute"`
	Memory            gcpMemory            `json:"memory"`
	Network           []gcpNetwork         `json:"network"`
	Swap              gcpSwap              `json:"swap"`
}

type gcpCPUUtilization struct {
	Total float64 `json:"total"`
	Idle  float64 `json:"idle"`
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

type gcpLoadAverageMinute struct {
	One     float64 `json:"one"`
	Five    float64 `json:"five"`
	Fifteen float64 `json:"fifteen"`
}

type gcpMemory struct {
	Total int64 `json:"total"`
	Free  int64 `json:"free"`
	Used  int64 `json:"used"`
}

type gcpNetwork struct {
	Interface string  `json:"interface"`
	Rx        float64 `json:"rx"`
	Tx        float64 `json:"tx"`
}

type gcpSwap struct {
	Total float64 `json:"total"`
	Free  float64 `json:"free"`
	Used  float64 `json:"used"`
}

var gcpAvailableMetrics = map[string]string{
	"cpu.utilization":        "cloudsql.googleapis.com/database/cpu/utilization",
	"memory.quota":           "cloudsql.googleapis.com/database/memory/quota",
	"memory.usage":           "cloudsql.googleapis.com/database/memory/usage",
	"swap.usage":             "cloudsql.googleapis.com/database/memory/swap_usage",
	"load.1m":                "cloudsql.googleapis.com/database/load_average/1m",
	"load.5m":                "cloudsql.googleapis.com/database/load_average/5m",
	"load.15m":               "cloudsql.googleapis.com/database/load_average/15m",
	"disk.read_ops":          "cloudsql.googleapis.com/database/disk/read_ops_count",
	"disk.write_ops":         "cloudsql.googleapis.com/database/disk/write_ops_count",
	"disk.read_bytes":        "cloudsql.googleapis.com/database/disk/read_bytes_count",
	"disk.write_bytes":       "cloudsql.googleapis.com/database/disk/write_bytes_count",
	"disk.bytes_used":        "cloudsql.googleapis.com/database/disk/bytes_used",
	"network.received_bytes": "cloudsql.googleapis.com/database/network/received_bytes_count",
	"network.sent_bytes":     "cloudsql.googleapis.com/database/network/sent_bytes_count",
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

	// Collect metrics from Cloud Monitoring
	gcpMetricsData, err := gcpmetrics.collectGCPMetrics()
	if err != nil {
		gcpmetrics.logger.Errorf("Failed to collect GCP metrics: %s", err)
		return err
	}

	// Build OS metrics structure
	osMetrics := gcpmetrics.buildOSMetrics(instance, gcpMetricsData)

	// Convert to JSON and back to simulate AWS processing
	osMetricsJSON, err := json.Marshal(osMetrics)
	if err != nil {
		gcpmetrics.logger.Errorf("Failed to marshal OS metrics: %s", err)
		return err
	}

	gcpmetrics.logger.V(5).Info("GCP OS Metrics JSON: ", string(osMetricsJSON))

	// Parse back from JSON
	var parsedMetrics gcpOSMetrics
	err = json.Unmarshal(osMetricsJSON, &parsedMetrics)
	if err != nil {
		gcpmetrics.logger.Errorf("Failed to unmarshal OS metrics: %s", err)
		return err
	}

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
	gcpmetrics.logger.V(5).Info("Swap ", parsedMetrics.Swap)

	// CPU Counts
	info["CPU"] = models.MetricGroupValue{"Counts": parsedMetrics.NumVCPUs}

	// DiskIO
	metricsMap["DiskIO"] = parsedMetrics.DiskIO
	gcpmetrics.logger.V(5).Info("DiskIO ", parsedMetrics.DiskIO)

	// CPU load average
	metricsMap["CPU"] = parsedMetrics.LoadAverageMinute
	gcpmetrics.logger.V(5).Info("CPU LoadAverage ", parsedMetrics.LoadAverageMinute)

	info["Host"] = models.MetricGroupValue{
		"InstanceType": parsedMetrics.InstanceTier,
		"Timestamp":    parsedMetrics.Timestamp,
		"Uptime":       parsedMetrics.Uptime,
		"Engine":       parsedMetrics.Engine,
		"Version":      parsedMetrics.Version,
	}

	metrics.System.Info = info
	metrics.System.Metrics = metricsMap
	gcpmetrics.logger.V(5).Info("CollectMetrics gcpcloudsqlenhancedmetrics ", metrics.System)

	return nil
}

func (gcpmetrics *GCPCloudSQLEnhancedMetricsGatherer) collectGCPMetrics() (map[string]float64, error) {
	ctx := context.Background()
	results := make(map[string]float64)

	// Collect recent metrics (excluding load averages)
	recentMetrics, err := gcpmetrics.collectRecentMetrics(ctx)
	if err != nil {
		return nil, err
	}

	// Merge recent metrics
	for key, value := range recentMetrics {
		results[key] = value
	}

	// Set load averages: use CPU utilization for 1m, others set to 0
	if cpuUtil, exists := results["cpu.utilization"]; exists {
		results["load.1m"] = cpuUtil / 100.0 // Convert percentage to 0-1 scale
	} else {
		results["load.1m"] = 0.0
	}
	results["load.5m"] = 0.0  // Set to 0 as requested
	results["load.15m"] = 0.0 // Set to 0 as requested

	return results, nil
}

// Request 1: Latest metrics (separate request for each metric type)
func (gcpmetrics *GCPCloudSQLEnhancedMetricsGatherer) collectRecentMetrics(ctx context.Context) (map[string]float64, error) {
	results := make(map[string]float64)

	// Metrics that need only the latest data point
	recentMetricTypes := []string{
		"cloudsql.googleapis.com/database/cpu/utilization",
		"cloudsql.googleapis.com/database/memory/quota",
		"cloudsql.googleapis.com/database/memory/usage",
		// "cloudsql.googleapis.com/database/memory/swap_usage", // Not available for Cloud SQL
		"cloudsql.googleapis.com/database/disk/read_ops_count",
		"cloudsql.googleapis.com/database/disk/write_ops_count",
		"cloudsql.googleapis.com/database/disk/read_bytes_count",
		"cloudsql.googleapis.com/database/disk/write_bytes_count",
		"cloudsql.googleapis.com/database/disk/bytes_used",
		"cloudsql.googleapis.com/database/network/received_bytes_count",
		"cloudsql.googleapis.com/database/network/sent_bytes_count",
	}

	// Make separate request for each metric type (GCP doesn't support OR on metric.type)
	for _, metricType := range recentMetricTypes {
		// GCP Cloud SQL database_id format is "project_id:instance_id"
		databaseID := fmt.Sprintf("%s:%s", gcpmetrics.projectID, gcpmetrics.instanceID)
		filter := fmt.Sprintf(`metric.type="%s" AND resource.label.database_id="%s"`, metricType, databaseID)

		// Use a time window that accounts for metric processing delays
		now := time.Now()
		endTime := now.Add(-30 * time.Second)      // End 30 seconds ago to allow for processing
		startTime := endTime.Add(-5 * time.Minute) // Look back 5 minutes from that point

		req := &monitoringpb.ListTimeSeriesRequest{
			Name:   fmt.Sprintf("projects/%s", gcpmetrics.projectID),
			Filter: filter,
			Interval: &monitoringpb.TimeInterval{
				EndTime:   timestamppb.New(endTime),
				StartTime: timestamppb.New(startTime),
			},
		}

		it := gcpmetrics.monitoringClient.ListTimeSeries(ctx, req)

		ts, err := it.Next()
		if err != nil {
			gcpmetrics.logger.V(5).Infof("No data for metric %s: %v", metricType, err)
			// Log the exact filter being used for debugging
			gcpmetrics.logger.V(5).Infof("Filter used: %s", filter)
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

	// Get memory from metrics (actual usage data)
	memoryUsage := int64(getMetric("memory.usage", 0))
	memoryQuota := int64(getMetric("memory.quota", 1073741824)) // 1GB default

	diskUsed := int64(getMetric("disk.bytes_used", 0))
	diskTotal := diskUsed * 2 // Estimate total
	if diskTotal == 0 {
		diskTotal = 1073741824 // 1GB default
	}

	// Extract engine name from database version
	engine := "mysql"
	if instance.DatabaseVersion != "" {
		if strings.Contains(strings.ToLower(instance.DatabaseVersion), "postgres") {
			engine = "postgres"
		}
	}

	// Get instance tier
	instanceTier := ""
	if instance.Settings != nil {
		instanceTier = instance.Settings.Tier
	}

	// Calculate CPU utilization safely
	cpuUtil := getMetric("cpu.utilization", 0)
	cpuIdle := 100 - cpuUtil
	if cpuIdle < 0 {
		cpuIdle = 0
	}

	return &gcpOSMetrics{
		Engine:             engine,
		InstanceID:         gcpmetrics.instanceID,
		InstanceResourceID: instance.Name,
		InstanceTier:       instanceTier,
		NumVCPUs:           0, // Platform will determine from tier
		Timestamp:          time.Now(),
		Uptime:             "0 days",
		Version:            1.0,

		CPUUtilization: gcpCPUUtilization{
			Total: cpuUtil,
			Idle:  cpuIdle,
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
				Name:        "cloudsql-disk",
				MountPoint:  "/var/lib/mysql",
				Used:        diskUsed,
				Total:       diskTotal,
				UsedPercent: float64(diskUsed) / float64(diskTotal) * 100,
			},
		},

		LoadAverageMinute: gcpLoadAverageMinute{
			One:     getMetric("load.1m", 0),
			Five:    getMetric("load.5m", 0),
			Fifteen: getMetric("load.15m", 0),
		},

		Memory: gcpMemory{
			Total: memoryQuota,
			Used:  memoryUsage,
			Free:  memoryQuota - memoryUsage,
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
			Free:  getMetric("memory.quota", 1073741824) - getMetric("swap.usage", 0),
		},
	}
}