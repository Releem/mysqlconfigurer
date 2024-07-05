package metrics

import (
	"context"
	"fmt"
	"time"

	"github.com/Releem/mysqlconfigurer/config"
	"github.com/advantageous/go-logback/logging"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/aws/aws-sdk-go/aws/awserr"
)

type AWSRDSMetricsGatherer struct {
	logger        logging.Logger
	debug         bool
	cwclient      *cloudwatch.Client
	configuration *config.Config
}

type rdsMetric struct {
	name string
}

var rdsMetrics = []rdsMetric{
	{name: "BinLogDiskUsage"},
	{name: "BurstBalance"},
	{name: "CPUUtilization"},
	{name: "CPUCreditUsage"},
	{name: "CPUCreditBalance"},
	{name: "CPUSurplusCreditBalance"},
	{name: "CPUSurplusCreditsCharged"},
	{name: "DatabaseConnections"},
	{name: "DiskQueueDepth"},
	{name: "FreeableMemory"},
	{name: "FreeStorageSpace"},
	{name: "LVMReadIOPS"},
	{name: "LVMWriteIOPS"},
	{name: "NetworkReceiveThroughput"},
	{name: "NetworkTransmitThroughput"},
	{name: "ReadIOPS"},
	{name: "ReadLatency"},
	{name: "ReadThroughput"},
	{name: "ReplicaLag"},
	{name: "SwapUsage"},
	{name: "WriteIOPS"},
	{name: "WriteLatency"},
	{name: "WriteThroughput"},
	{name: "NumVCPUs"},
}

func NewAWSRDSMetricsGatherer(logger logging.Logger, cwclient *cloudwatch.Client, configuration *config.Config) *AWSRDSMetricsGatherer {

	if logger == nil {
		if configuration.Debug {
			logger = logging.NewSimpleDebugLogger("AWSMetrics")
		} else {
			logger = logging.NewSimpleLogger("AWSMetrics")
		}
	}

	return &AWSRDSMetricsGatherer{
		logger:        logger,
		debug:         configuration.Debug,
		cwclient:      cwclient,
		configuration: configuration,
	}
}

func (awsrdsmetrics *AWSRDSMetricsGatherer) GetMetrics(metrics *Metrics) error {
	defer HandlePanic(awsrdsmetrics.configuration, awsrdsmetrics.logger)

	MetricDataQueries := []types.MetricDataQuery{}
	output := make(MetricGroupValue)

	// Prepare request to CloudWatch
	for _, metric := range rdsMetrics {
		MetricDataQueries = append(MetricDataQueries,
			types.MetricDataQuery{
				Id: aws.String("id" + metric.name),
				MetricStat: &types.MetricStat{
					Metric: &types.Metric{
						Namespace:  aws.String("AWS/RDS"),
						MetricName: aws.String(metric.name),
						Dimensions: []types.Dimension{
							{
								Name:  aws.String("DBInstanceIdentifier"),
								Value: aws.String(awsrdsmetrics.configuration.AwsRDSDB),
							},
						},
					},
					Period: aws.Int32(60),
					Stat:   aws.String("Average"),
				},
			})
	}

	input := &cloudwatch.GetMetricDataInput{
		EndTime:           aws.Time(time.Unix(time.Now().Unix(), 0)),
		StartTime:         aws.Time(time.Unix(time.Now().Add(time.Duration(-2)*time.Minute).Unix(), 0)),
		MetricDataQueries: MetricDataQueries,
	}

	// Request to CloudWatch
	result, err := awsrdsmetrics.cwclient.GetMetricData(context.TODO(), input)

	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			awsrdsmetrics.logger.Error(aerr.Error())

		} else {
			// Print the error, cast err to awserr.Error to get the Code and
			// Message from an error.
			awsrdsmetrics.logger.Error(err.Error())
		}
	} else {
		awsrdsmetrics.logger.Println("CloudWatch.GetMetricData SUCCESS")
	}

	// Prepare results
	for _, r := range result.MetricDataResults {
		awsrdsmetrics.logger.Debug("Metric ID ", *r.Id)
		awsrdsmetrics.logger.Debug("Metric Label ", *r.Label)

		if len(r.Values) > 0 {
			output[*r.Label] = fmt.Sprintf("%f", r.Values[0])
			awsrdsmetrics.logger.Debug("Metric Timestamp ", r.Timestamps[0])
		} else {
			awsrdsmetrics.logger.Debug("CloudWatch.GetMetricData no Values for ", *r.Label)
		}
	}
	// temperary
	metrics.System.Metrics = output
	awsrdsmetrics.logger.Debug("collectMetrics ", metrics.System.Metrics)
	return nil

}
