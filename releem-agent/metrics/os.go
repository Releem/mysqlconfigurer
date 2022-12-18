package metrics

import (
	"encoding/json"
	"strings"

	"github.com/Releem/mysqlconfigurer/releem-agent/config"
	"github.com/advantageous/go-logback/logging"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/load"
	"github.com/shirou/gopsutil/v3/mem"
)

type OSMetricsGatherer struct {
	logger        logging.Logger
	debug         bool
	configuration *config.Config
}

func NewOSMetricsGatherer(logger logging.Logger, configuration *config.Config) *OSMetricsGatherer {

	if logger == nil {
		if configuration.Debug {
			logger = logging.NewSimpleDebugLogger("OS")
		} else {
			logger = logging.NewSimpleLogger("OS")
		}
	}

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

func StructToMap(valueStruct string) MetricGroupValue {
	var value_map MetricGroupValue

	_ = json.Unmarshal([]byte(valueStruct), &value_map)
	return value_map
}
func (OS *OSMetricsGatherer) GetMetrics(metrics *Metrics) error {

	info := make(MetricGroupValue)

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
	//info["VirtualMemory"] = StructToMap(VirtualMemory.String())
	metrics.System.Metrics.PhysicalMemory = StructToMap(VirtualMemory.String())

	//CPU Counts
	CpuCounts, _ := cpu.Counts(true)
	info["CPU"] = MetricGroupValue{"Counts": CpuCounts}

	//OS host info
	hostInfo, _ := host.Info()
	hostInfoMap := StructToMap(hostInfo.String())
	hostInfoMap["InstanceType"] = "local"
	info["Host"] = hostInfoMap

	// IOCounters, _ := disk.IOCounters()
	// //info["IOCounters"] = StructToMap(IOCounters.String())
	// OS.logger.Debug("IOCounters ", IOCounters)

	//Get partitions, for each pert get usage and io stat
	var UsageArray, PartitionsArray, IOCountersArray []MetricGroupValue
	var readCount, writeCount uint64
	//:= make(MetricGroupValue)
	Partitions, _ := disk.Partitions(false)
	for _, part := range Partitions {
		Usage, _ := disk.Usage(part.Mountpoint)
		UsageArray = append(UsageArray, StructToMap(Usage.String()))
		PartitionsArray = append(PartitionsArray, StructToMap(part.String()))
		PartName := part.Device[strings.LastIndex(part.Device, "/")+1:]
		IOCounters, _ := disk.IOCounters(PartName)
		readCount = readCount + IOCounters[PartName].ReadCount
		writeCount = writeCount + IOCounters[PartName].WriteCount
		OS.logger.Debug("IOCounters ", IOCounters)
		IOCountersArray = append(IOCountersArray, MetricGroupValue{PartName: StructToMap(IOCounters[PartName].String())})
	}
	info["Partitions"] = PartitionsArray
	OS.logger.Debug("Partitions ", PartitionsArray)

	// info["Usage"] = UsageArray
	metrics.System.Metrics.FileSystem = UsageArray
	OS.logger.Debug("Usage ", UsageArray)

	metrics.System.Metrics.DiskIO = IOCountersArray
	OS.logger.Debug("IOCountersArray ", IOCountersArray)

	// CPU load avarage
	Avg, _ := load.Avg()
	metrics.System.Metrics.CPU = StructToMap(Avg.String())
	OS.logger.Debug("Avg ", Avg)

	// CpuUtilisation := float64(metrics.System.Metrics.CPU["load1"].(float64) / float64(info["CPU"].(MetricGroupValue)["Counts"].(int)))
	// metrics.System.Metrics.CPU["CpuUtilisation"] = CpuUtilisation
	// info["Cpu"] = MetricGroupValue{"CpuUtilisation": (info["Avg"].(MetricGroupValue)["load1"].(float64) / float64(info["Cpu"].(MetricGroupValue)["Counts"].(int)))}

	//Calc iops read and write as io count / uptime
	metrics.System.Metrics.IOPS = MetricGroupValue{"IOPSRead": (float64(readCount) / info["Host"].(MetricGroupValue)["uptime"].(float64)), "IOPSWrite": (float64(writeCount) / info["Host"].(MetricGroupValue)["uptime"].(float64))}

	metrics.System.Info = info

	OS.logger.Debug("collectMetrics ", metrics.System)
	return nil

}
