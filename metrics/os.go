package metrics

import (
	"encoding/json"
	"strings"

	"github.com/Releem/mysqlconfigurer/config"
	"github.com/Releem/mysqlconfigurer/models"
	"github.com/Releem/mysqlconfigurer/utils"
	logging "github.com/google/logger"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"
)

type OSMetricsGatherer struct {
	logger        logging.Logger
	debug         bool
	configuration *config.Config
}

func NewOSMetricsGatherer(logger logging.Logger, configuration *config.Config) *OSMetricsGatherer {
	return &OSMetricsGatherer{
		logger:        logger,
		debug:         configuration.Debug,
		configuration: configuration,
	}
}

// func cpu_cores(os_type string) string {
// 	if os_type == "Linux" {
// 		cntCPU, _ := exec.Command("awk -F: '/^core id/ && !P[$2] { CORES++; P[$2]=1 }; /^physical id/ && !N[$2] { CPUs++; N[$2]=1 };  END { print CPUs*CORES }' /proc/cpuinfo").Output()
// 		cntCPU_1 := strings.Trim(string(cntCPU), "\n")
// 		if cntCPU_1 == "0" {
// 			out, _ := exec.Command("nproc").Output()
// 			// string to int
// 			i, err := strconv.Atoi(string(out))
// 			if err != nil {
// 				return strconv.Itoa(0)
// 			}
// 			return strconv.Itoa(i)
// 		} else {
// 			i, err := strconv.Atoi(string(cntCPU_1))
// 			if err != nil {
// 				return strconv.Itoa(0)
// 			}
// 			return strconv.Itoa(i)
// 		}
// 	}
// 	if os_type == "FreeBSD" {
// 		cntCPU, _ := exec.Command("sysctl -n kern.smp.cores").Output()
// 		cntCPU_1 := strings.Trim(string(cntCPU), "\n")
// 		i, err := strconv.Atoi(string(cntCPU_1))
// 		if err != nil {
// 			return strconv.Itoa(0)
// 		}
// 		return strconv.Itoa(i + 1)
// 	}

// 	return strconv.Itoa(0)
// }

// func is_virtual_machine(os_type string) int {
// 	if os_type == "Linux" {
// 		isVm, _ := exec.Command("grep -Ec '^flags.* hypervisor ' /proc/cpuinfo").Output()
// 		if string(isVm) == "0" {
// 			return 0
// 		} else {
// 			return 1
// 		}
// 	}
// 	if os_type == "FreeBSD" {
// 		isVm, _ := exec.Command("sysctl -n kern.vm_guest").Output()
// 		isVm_1 := strings.Trim(string(isVm), "\n")
// 		if isVm_1 == "none" {
// 			return 0
// 		} else {
// 			return 1
// 		}
// 	}

// 	return 0
// }

func StructToMap(valueStruct string) models.MetricGroupValue {
	var value_map models.MetricGroupValue

	_ = json.Unmarshal([]byte(valueStruct), &value_map)
	return value_map
}
func (OS *OSMetricsGatherer) GetMetrics(metrics *models.Metrics) error {
	defer utils.HandlePanic(OS.configuration, OS.logger)
	info := make(models.MetricGroupValue)
	metricsMap := make(models.MetricGroupValue)

	// if out, err := exec.Command("uname").Output(); err != nil {
	// 	return err
	// } else {
	// 	info["OS Type"] = strings.Trim(string(out), "\n")
	// }
	//output["Physical Memory"] = make(map[string]string)
	// if forcemem := OS.configuration.GetMemoryLimit(); forcemem == 0 {
	// 	virtualMemory, _ := mem.VirtualMemory()
	// 	output["Physical Memory"] = map[string]uint64{"bytes": uint64(virtualMemory.Total)}
	// } else {
	// 	output["Physical Memory"] = map[string]uint64{"bytes": uint64(forcemem * 1048576)}
	// }

	// OS RAM
	VirtualMemory, _ := mem.VirtualMemory()
	metricsMap["PhysicalMemory"] = StructToMap(VirtualMemory.String())
	info["PhysicalMemory"] = models.MetricGroupValue{"total": VirtualMemory.Total}

	// OS SwapMemory
	SwapMemory, _ := mem.SwapMemory()
	metricsMap["SwapMemory"] = StructToMap(SwapMemory.String())
	info["PhysicalMemory"] = utils.MapJoin(info["PhysicalMemory"].(models.MetricGroupValue), models.MetricGroupValue{"swapTotal": SwapMemory.Total})

	//CPU Counts
	CpuCounts, _ := cpu.Counts(true)
	info["CPU"] = models.MetricGroupValue{"Counts": CpuCounts}

	//OS host info
	hostInfo, _ := host.Info()
	hostInfoMap := utils.MapJoin(StructToMap(hostInfo.String()), models.MetricGroupValue{"InstanceType": "local"})
	info["Host"] = hostInfoMap

	//Get partitions, for each pert get usage and io stat
	var UsageArray, PartitionsArray, IOCountersArray []models.MetricGroupValue
	var readCount, writeCount uint64
	//:= make(models.MetricGroupValue)
	PartitionCheck := make(map[string]int)
	Partitions, err := disk.Partitions(false)
	if err != nil {
		OS.logger.Error(err)
	}
	for _, part := range Partitions {
		Usage, err := disk.Usage(part.Mountpoint)
		if err != nil {
			OS.logger.Error(err)
		} else {
			UsageArray = append(UsageArray, StructToMap(Usage.String()))
		}
		PartitionsArray = append(PartitionsArray, StructToMap(part.String()))
		PartName := part.Device[strings.LastIndex(part.Device, "/")+1:]
		IOCounters, err := disk.IOCounters(PartName)
		if err != nil {
			OS.logger.Error(err)
		} else {
			if _, exists := PartitionCheck[part.Device]; !exists {

				readCount = readCount + IOCounters[PartName].ReadCount
				writeCount = writeCount + IOCounters[PartName].WriteCount
				PartitionCheck[part.Device] = 1
			}
			OS.logger.V(5).Info("IOCounters ", IOCounters)
			IOCountersArray = append(IOCountersArray, models.MetricGroupValue{PartName: StructToMap(IOCounters[PartName].String())})
		}
	}
	OS.logger.V(5).Info("PartitionCheck ", PartitionCheck)
	info["Partitions"] = PartitionsArray
	OS.logger.V(5).Info("Partitions ", PartitionsArray)

	// info["Usage"] = UsageArray
	metricsMap["FileSystem"] = UsageArray
	OS.logger.V(5).Info("Usage ", UsageArray)

	metricsMap["DiskIO"] = IOCountersArray
	OS.logger.V(5).Info("IOCountersArray ", IOCountersArray)

	// CPU load avarage
	Avg, _ := load.Avg()
	metricsMap["CPU"] = StructToMap(Avg.String())
	OS.logger.V(5).Info("Avg ", Avg)

	// CpuUtilisation := float64(metrics.System.Metrics.CPU["load1"].(float64) / float64(info["CPU"].(models.MetricGroupValue)["Counts"].(int)))
	// metrics.System.Metrics.CPU["CpuUtilisation"] = CpuUtilisation
	// info["Cpu"] = models.MetricGroupValue{"CpuUtilisation": (info["Avg"].(models.MetricGroupValue)["load1"].(float64) / float64(info["Cpu"].(models.MetricGroupValue)["Counts"].(int)))}

	//Calc iops read and write as io count / uptime
	metricsMap["IOP"] = models.MetricGroupValue{"IOPRead": float64(readCount), "IOPWrite": float64(writeCount)}

	metrics.System.Info = info
	metrics.System.Metrics = metricsMap
	OS.logger.V(5).Info("CollectMetrics OS ", metrics.System)

	return nil
}
