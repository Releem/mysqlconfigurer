package metrics

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/Releem/mysqlconfigurer/config"
	"github.com/advantageous/go-logback/logging"

	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/rds/types"
	"github.com/aws/aws-sdk-go/aws"
)

const rdsMetricsLogGroupName = "RDSOSMetrics"

type AWSRDSEnhancedMetricsGatherer struct {
	logger        logging.Logger
	debug         bool
	dbinstance    types.DBInstance
	cwlogsclient  *cloudwatchlogs.Client
	configuration *config.Config
}

type osMetrics struct {
	Engine             string    `json:"engine"             help:"The database engine for the DB instance."`
	InstanceID         string    `json:"instanceID"         help:"The DB instance identifier."`
	InstanceResourceID string    `json:"instanceResourceID" help:"A region-unique, immutable identifier for the DB instance, also used as the log stream identifier."`
	NumVCPUs           int       `json:"numVCPUs"           help:"The number of virtual CPUs for the DB instance."`
	Timestamp          time.Time `json:"timestamp"          help:"The time at which the metrics were taken."`
	Uptime             string    `json:"uptime"             help:"The amount of time that the DB instance has been active."`
	Version            float64   `json:"version"            help:"The version of the OS metrics' stream JSON format."`

	CPUUtilization    cpuUtilization    `json:"cpuUtilization"`
	DiskIO            []diskIO          `json:"diskIO"`
	FileSys           []fileSys         `json:"fileSys"`
	LoadAverageMinute loadAverageMinute `json:"loadAverageMinute"`
	Memory            memory            `json:"memory"`
	Network           []network         `json:"network"`
	ProcessList       []processList     `json:"processList"`
	Swap              swap              `json:"swap"`
	Tasks             tasks             `json:"tasks"`

	// TODO Handle this: https://jira.percona.com/browse/PMM-3835
	PhysicalDeviceIO []diskIO `json:"physicalDeviceIO"`
}

type cpuUtilization struct {
	Guest  float64 `json:"guest"  help:"The percentage of CPU in use by guest programs."`
	Idle   float64 `json:"idle"   help:"The percentage of CPU that is idle."`
	Irq    float64 `json:"irq"    help:"The percentage of CPU in use by software interrupts."`
	Nice   float64 `json:"nice"   help:"The percentage of CPU in use by programs running at lowest priority."`
	Steal  float64 `json:"steal"  help:"The percentage of CPU in use by other virtual machines."`
	System float64 `json:"system" help:"The percentage of CPU in use by the kernel."`
	Total  float64 `json:"total"  help:"The total percentage of the CPU in use. This value includes the nice value."`
	User   float64 `json:"user"   help:"The percentage of CPU in use by user programs."`
	Wait   float64 `json:"wait"   help:"The percentage of CPU unused while waiting for I/O access."`
}

//nolint:lll
type diskIO struct {
	// common
	ReadIOsPS  float64 `json:"readIOsPS"  help:"The number of read operations per second."`
	WriteIOsPS float64 `json:"writeIOsPS" help:"The number of write operations per second."`
	Device     string  `json:"device"     help:"The identifier of the disk device in use."`

	// non-Aurora
	AvgQueueLen *float64 `json:"avgQueueLen" help:"The number of requests waiting in the I/O device's queue."`
	AvgReqSz    *float64 `json:"avgReqSz"    help:"The average request size, in kilobytes."`
	Await       *float64 `json:"await"       help:"The number of milliseconds required to respond to requests, including queue time and service time."`
	ReadKb      *int     `json:"readKb"      help:"The total number of kilobytes read."`
	ReadKbPS    *float64 `json:"readKbPS"    help:"The number of kilobytes read per second."`
	RrqmPS      *float64 `json:"rrqmPS"      help:"The number of merged read requests queued per second."`
	TPS         *float64 `json:"tps"         help:"The number of I/O transactions per second."`
	Util        *float64 `json:"util"        help:"The percentage of CPU time during which requests were issued."`
	WriteKb     *int     `json:"writeKb"     help:"The total number of kilobytes written."`
	WriteKbPS   *float64 `json:"writeKbPS"   help:"The number of kilobytes written per second."`
	WrqmPS      *float64 `json:"wrqmPS"      help:"The number of merged write requests queued per second."`

	// Aurora
	DiskQueueDepth  *float64 `json:"diskQueueDepth"  help:"The number of outstanding IOs (read/write requests) waiting to access the disk."`
	ReadLatency     *float64 `json:"readLatency"     help:"The average amount of time taken per disk I/O operation."`
	ReadThroughput  *float64 `json:"readThroughput"  help:"The average number of bytes read from disk per second."`
	WriteLatency    *float64 `json:"writeLatency"    help:"The average amount of time taken per disk I/O operation."`
	WriteThroughput *float64 `json:"writeThroughput" help:"The average number of bytes written to disk per second."`
}

//nolint:lll
type fileSys struct {
	MaxFiles        int     `json:"maxFiles"        help:"The maximum number of files that can be created for the file system."`
	MountPoint      string  `json:"mountPoint"      help:"The path to the file system."`
	Name            string  `json:"name"            help:"The name of the file system."`
	Total           int     `json:"total"           help:"The total number of disk space available for the file system, in kilobytes."`
	Used            int     `json:"used"            help:"The amount of disk space used by files in the file system, in kilobytes."`
	UsedFilePercent float64 `json:"usedFilePercent" help:"The percentage of available files in use."`
	UsedFiles       int     `json:"usedFiles"       help:"The number of files in the file system."`
	UsedPercent     float64 `json:"usedPercent"     help:"The percentage of the file-system disk space in use."`
}

type loadAverageMinute struct {
	Fifteen float64 `json:"fifteen" help:"The number of processes requesting CPU time over the last 15 minutes."`
	Five    float64 `json:"five"    help:"The number of processes requesting CPU time over the last 5 minutes."`
	One     float64 `json:"one"     help:"The number of processes requesting CPU time over the last minute."`
}

//nolint:lll
type memory struct {
	Active         int `json:"active"         node:"Active_bytes"       m:"1024" help:"The amount of assigned memory, in kilobytes."`
	Buffers        int `json:"buffers"        node:"Buffers_bytes"      m:"1024" help:"The amount of memory used for buffering I/O requests prior to writing to the storage device, in kilobytes."`
	Cached         int `json:"cached"         node:"Cached_bytes"       m:"1024" help:"The amount of memory used for caching file system–based I/O."`
	Dirty          int `json:"dirty"          node:"Dirty_bytes"        m:"1024" help:"The amount of memory pages in RAM that have been modified but not written to their related data block in storage, in kilobytes."`
	Free           int `json:"free"           node:"MemFree_bytes"      m:"1024" help:"The amount of unassigned memory, in kilobytes."`
	HugePagesFree  int `json:"hugePagesFree"  node:"HugePages_Free"     m:"1"    help:"The number of free huge pages. Huge pages are a feature of the Linux kernel."`
	HugePagesRsvd  int `json:"hugePagesRsvd"  node:"HugePages_Rsvd"     m:"1"    help:"The number of committed huge pages."`
	HugePagesSize  int `json:"hugePagesSize"  node:"Hugepagesize_bytes" m:"1024" help:"The size for each huge pages unit, in kilobytes."`
	HugePagesSurp  int `json:"hugePagesSurp"  node:"HugePages_Surp"     m:"1"    help:"The number of available surplus huge pages over the total."`
	HugePagesTotal int `json:"hugePagesTotal" node:"HugePages_Total"    m:"1"    help:"The total number of huge pages for the system."`
	Inactive       int `json:"inactive"       node:"Inactive_bytes"     m:"1024" help:"The amount of least-frequently used memory pages, in kilobytes."`
	Mapped         int `json:"mapped"         node:"Mapped_bytes"       m:"1024" help:"The total amount of file-system contents that is memory mapped inside a process address space, in kilobytes."`
	PageTables     int `json:"pageTables"     node:"PageTables_bytes"   m:"1024" help:"The amount of memory used by page tables, in kilobytes."`
	Slab           int `json:"slab"           node:"Slab_bytes"         m:"1024" help:"The amount of reusable kernel data structures, in kilobytes."`
	Total          int `json:"total"          node:"MemTotal_bytes"     m:"1024" help:"The total amount of memory, in kilobytes."`
	Writeback      int `json:"writeback"      node:"Writeback_bytes"    m:"1024" help:"The amount of dirty pages in RAM that are still being written to the backing storage, in kilobytes."`
}

type network struct {
	Interface string  `json:"interface" help:"The identifier for the network interface being used for the DB instance."`
	Rx        float64 `json:"rx"        help:"The number of bytes received per second."`
	Tx        float64 `json:"tx"        help:"The number of bytes uploaded per second."`
}

//nolint:lll
type processList struct {
	CPUUsedPC    float64 `json:"cpuUsedPc"    help:"The percentage of CPU used by the process."`
	ID           int     `json:"id"           help:"The identifier of the process."`
	MemoryUsedPC float64 `json:"memoryUsedPc" help:"The amount of memory used by the process, in kilobytes."`
	Name         string  `json:"name"         help:"The name of the process."`
	ParentID     int     `json:"parentID"     help:"The process identifier for the parent process of the process."`
	RSS          int     `json:"rss"          help:"The amount of RAM allocated to the process, in kilobytes."`
	TGID         int     `json:"tgid"         help:"The thread group identifier, which is a number representing the process ID to which a thread belongs. This identifier is used to group threads from the same process."`
	VSS          int     `json:"vss"          help:"The amount of virtual memory allocated to the process, in kilobytes."`

	// TODO Handle this: https://jira.percona.com/browse/PMM-5150
	VMLimit interface{} `json:"vmlimit" help:"-"`
}

//nolint:lll
type swap struct {
	Cached float64 `json:"cached" node:"node_memory_SwapCached_bytes" m:"1024" help:"The amount of swap memory, in kilobytes, used as cache memory."  nodehelp:"Memory information field SwapCached."`
	Free   float64 `json:"free"   node:"node_memory_SwapFree_bytes"   m:"1024" help:"The total amount of swap memory free, in kilobytes."             nodehelp:"Memory information field SwapFree."`
	Total  float64 `json:"total"  node:"node_memory_SwapTotal_bytes"  m:"1024" help:"The total amount of swap memory available, in kilobytes."        nodehelp:"Memory information field SwapTotal."`

	// we use multiplier 0.25 to convert a number of kilobytes to a number of 4k pages (what our dashboards assume)
	In  float64 `json:"in"  node:"node_vmstat_pswpin"  m:"0.25" help:"The total amount of memory, in kilobytes, swapped in from disk." nodehelp:"/proc/vmstat information field pswpin"`
	Out float64 `json:"out" node:"node_vmstat_pswpout" m:"0.25" help:"The total amount of memory, in kilobytes, swapped out to disk."  nodehelp:"/proc/vmstat information field pswpout"`
}

type tasks struct {
	Blocked  int `json:"blocked"  help:"The number of tasks that are blocked."`
	Running  int `json:"running"  help:"The number of tasks that are running."`
	Sleeping int `json:"sleeping" help:"The number of tasks that are sleeping."`
	Stopped  int `json:"stopped"  help:"The number of tasks that are stopped."`
	Total    int `json:"total"    help:"The total number of tasks."`
	Zombie   int `json:"zombie"   help:"The number of child tasks that are inactive with an active parent task."`
}

// parseOSMetrics parses OS metrics from given JSON data.
func parseOSMetrics(b []byte, disallowUnknownFields bool) (*osMetrics, error) {
	d := json.NewDecoder(bytes.NewReader(b))
	if disallowUnknownFields {
		d.DisallowUnknownFields()
	}

	var m osMetrics
	if err := d.Decode(&m); err != nil {
		return nil, err
	}
	return &m, nil
}

func NewAWSRDSEnhancedMetricsGatherer(logger logging.Logger, dbinstance types.DBInstance, cwlogsclient *cloudwatchlogs.Client, configuration *config.Config) *AWSRDSEnhancedMetricsGatherer {

	if logger == nil {
		if configuration.Debug {
			logger = logging.NewSimpleDebugLogger("AWSEnhancedMetrics")
		} else {
			logger = logging.NewSimpleLogger("AWSEnhancedMetrics")
		}
	}

	return &AWSRDSEnhancedMetricsGatherer{
		logger:        logger,
		debug:         configuration.Debug,
		cwlogsclient:  cwlogsclient,
		dbinstance:    dbinstance,
		configuration: configuration,
	}
}

func (awsrdsenhancedmetrics *AWSRDSEnhancedMetricsGatherer) GetMetrics(metrics *Metrics) error {
	defer HandlePanic(awsrdsenhancedmetrics.configuration, awsrdsenhancedmetrics.logger)

	info := make(MetricGroupValue)
	metricsMap := make(MetricGroupValue)

	input := cloudwatchlogs.GetLogEventsInput{
		Limit:         aws.Int32(1),
		StartFromHead: aws.Bool(false),
		LogGroupName:  aws.String(rdsMetricsLogGroupName),
		LogStreamName: awsrdsenhancedmetrics.dbinstance.DbiResourceId,
	}

	result, err := awsrdsenhancedmetrics.cwlogsclient.GetLogEvents(context.TODO(), &input)

	if err != nil {
		awsrdsenhancedmetrics.logger.Critical("failed to read log stream %s:%s: %s", rdsMetricsLogGroupName, aws.StringValue(awsrdsenhancedmetrics.dbinstance.DbiResourceId), err)
		return err
	}

	awsrdsenhancedmetrics.logger.Debug("CloudWatchLogs.GetLogEvents SUCCESS")

	if len(result.Events) < 1 {
		awsrdsenhancedmetrics.logger.Warn("CloudWatchLogs.GetLogEvents No data")
		return errors.New("CloudWatchLogs.GetLogEvents No data")
	}

	// l.Debugf("Message:\n%s", *event.Message)
	osMetrics, err := parseOSMetrics([]byte(*result.Events[0].Message), true)

	if err != nil {
		awsrdsenhancedmetrics.logger.Errorf("Failed to parse metrics: %s.", err)
		return err
	}

	// Set IOPS
	var readCount, writeCount float64

	for _, diskio := range osMetrics.DiskIO {
		readCount = readCount + diskio.ReadIOsPS
		writeCount = writeCount + diskio.WriteIOsPS
	}

	metricsMap["IOP"] = MetricGroupValue{"IOPRead": readCount, "IOPWrite": writeCount}

	// Set FileSystem
	metricsMap["FileSystem"] = osMetrics.FileSys

	// OS RAM
	metricsMap["PhysicalMemory"] = osMetrics.Memory
	info["PhysicalMemory"] = MetricGroupValue{"total": osMetrics.Memory.Total}
	info["PhysicalMemory"] = MapJoin(info["PhysicalMemory"].(MetricGroupValue), MetricGroupValue{"swapTotal": osMetrics.Swap.Total})

	// Swap
	metricsMap["Swap"] = osMetrics.Swap
	awsrdsenhancedmetrics.logger.Debug("Swap ", osMetrics.Swap)

	//CPU Counts
	info["CPU"] = MetricGroupValue{"Counts": osMetrics.NumVCPUs}

	// FileSys
	metricsMap["FileSystem"] = osMetrics.FileSys
	awsrdsenhancedmetrics.logger.Debug("FileSystem ", osMetrics.FileSys)

	//DiskIO
	metricsMap["DiskIO"] = osMetrics.DiskIO
	awsrdsenhancedmetrics.logger.Debug("DiskIO ", osMetrics.DiskIO)

	// CPU load avarage
	metricsMap["CPU"] = osMetrics.LoadAverageMinute //StructToMap(Avg.String())
	awsrdsenhancedmetrics.logger.Debug("CPU ", osMetrics.LoadAverageMinute)

	info["Host"] = MetricGroupValue{
		"InstanceType": "aws/rds",
		"Timestamp":    osMetrics.Timestamp,
		"Uptime":       osMetrics.Uptime,
		"Engine":       osMetrics.Engine,
		"Version":      osMetrics.Version,
	}

	metrics.System.Info = info
	metrics.System.Metrics = metricsMap
	awsrdsenhancedmetrics.logger.Debug("collectMetrics ", metrics.System)

	return nil

}
