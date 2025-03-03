package metrics

import (
	"context"
	"strings"

	"github.com/Releem/mysqlconfigurer/config"
	"github.com/Releem/mysqlconfigurer/models"
	"github.com/Releem/mysqlconfigurer/utils"
	logging "github.com/google/logger"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	"github.com/aws/aws-sdk-go/aws/awserr"
)

type AWSRDSInstanceGatherer struct {
	logger        logging.Logger
	debug         bool
	rdsclient     *rds.Client
	ec2client     *ec2.Client
	configuration *config.Config
}

func NewAWSRDSInstanceGatherer(logger logging.Logger, rdsclient *rds.Client, ec2client *ec2.Client, configuration *config.Config) *AWSRDSInstanceGatherer {
	return &AWSRDSInstanceGatherer{
		logger:        logger,
		debug:         configuration.Debug,
		rdsclient:     rdsclient,
		ec2client:     ec2client,
		configuration: configuration,
	}
}

func (awsrdsinstance *AWSRDSInstanceGatherer) GetMetrics(metrics *models.Metrics) error {
	defer utils.HandlePanic(awsrdsinstance.configuration, awsrdsinstance.logger)

	//output := make(models.MetricGroupValue)
	info := make(models.MetricGroupValue)

	// Prepare request to RDS
	input := &rds.DescribeDBInstancesInput{
		DBInstanceIdentifier: &awsrdsinstance.configuration.AwsRDSDB,
	}

	// Request to RDS
	result, err := awsrdsinstance.rdsclient.DescribeDBInstances(context.TODO(), input)

	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			awsrdsinstance.logger.Error(aerr.Error())

		} else {
			// Print the error, cast err to awserr.Error to get the Code and
			// Message from an error.
			awsrdsinstance.logger.Error(err.Error())
		}
	} else {
		awsrdsinstance.logger.Info("RDS.DescribeDBInstances SUCCESS")

		// Request detailed instance info
		if len(result.DBInstances) == 1 {

			r := result.DBInstances[0]

			awsrdsinstance.logger.V(5).Info("DBInstance ", r.DBInstanceIdentifier)
			awsrdsinstance.logger.V(5).Info("DBInstanceClass ", r.DBInstanceClass)
			awsrdsinstance.logger.V(5).Info("ProcessorFeatures ", r.ProcessorFeatures)

			// Prepare request to Ec2 to determine CPU count and Ram for InstanceClass
			instanceName := strings.TrimPrefix(*r.DBInstanceClass, "db.")
			ec2input := &ec2.DescribeInstanceTypesInput{
				InstanceTypes: []types.InstanceType{types.InstanceType(instanceName)},
			}

			// Request to EC2 to get Type info
			typeinfo, infoerr := awsrdsinstance.ec2client.DescribeInstanceTypes(context.TODO(), ec2input)

			if infoerr != nil {
				if aerr, ok := infoerr.(awserr.Error); ok {
					awsrdsinstance.logger.Error(aerr.Error())

				} else {
					// Print the error, cast err to awserr.Error to get the Code and
					// Message from an error.
					awsrdsinstance.logger.Error(infoerr.Error())
				}
			} else {
				awsrdsinstance.logger.V(5).Infof("EC2.DescribeInstanceType SUCCESS")
				awsrdsinstance.logger.V(5).Infof("EC2.DescribeInstanceType %v", typeinfo)
			}

			if len(typeinfo.InstanceTypes) > 0 {
				info["CPU"] = models.MetricGroupValue{"Counts": typeinfo.InstanceTypes[0].VCpuInfo.DefaultVCpus}
				info["PhysicalMemory"] = models.MetricGroupValue{"total": *typeinfo.InstanceTypes[0].MemoryInfo.SizeInMiB * 1024 * 1024}
			}

			info["Host"] = models.MetricGroupValue{"InstanceType": "aws/rds"}

		} else if len(result.DBInstances) > 1 {
			awsrdsinstance.logger.Infof("RDS.DescribeDBInstances: Database has %d instances. Clusters are not supported", len(result.DBInstances))
		} else {
			awsrdsinstance.logger.Info("RDS.DescribeDBInstances: No instances")
		}

	}

	metrics.System.Info = info
	awsrdsinstance.logger.V(5).Info("CollectMetrics awsrdsinstance", info)
	return nil

}
