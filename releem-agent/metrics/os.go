package metrics

import (
	"os/exec"
	"strconv"
	"strings"

	"github.com/Releem/mysqlconfigurer/releem-agent/config"
	"github.com/advantageous/go-logback/logging"
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

func cpu_cores(os_type string) int {
	if os_type == "Linux" {
		cntCPU, _ := exec.Command("awk -F: '/^core id/ && !P[$2] { CORES++; P[$2]=1 }; /^physical id/ && !N[$2] { CPUs++; N[$2]=1 };  END { print CPUs*CORES }' /proc/cpuinfo").Output()
		cntCPU_1 := strings.Trim(string(cntCPU), "\n")
		if cntCPU_1 == "0" {
			out, _ := exec.Command("nproc").Output()
			// string to int
			i, err := strconv.Atoi(string(out))
			if err != nil {
				return 0
			}
			return i
		} else {
			i, err := strconv.Atoi(string(cntCPU_1))
			if err != nil {
				return 0
			}
			return i
		}
	}
	if os_type == "FreeBSD" {
		cntCPU, _ := exec.Command("sysctl -n kern.smp.cores").Output()
		cntCPU_1 := strings.Trim(string(cntCPU), "\n")
		i, err := strconv.Atoi(string(cntCPU_1))
		if err != nil {
			return 0
		}
		return i + 1
	}

	return 0
}

func is_virtual_machine(os_type string) int {
	if os_type == "Linux" {
		isVm, _ := exec.Command("grep -Ec '^flags.* hypervisor ' /proc/cpuinfo").Output()
		if string(isVm) == "0" {
			return 0
		} else {
			return 1
		}
	}
	if os_type == "FreeBSD" {
		isVm, _ := exec.Command("sysctl -n kern.vm_guest").Output()
		isVm_1 := strings.Trim(string(isVm), "\n")
		if isVm_1 == "none" {
			return 0
		} else {
			return 1
		}
	}

	return 0
}

func (OS *OSMetricsGatherer) GetMetrics() (Metric, error) {

	output := make(map[string]interface{})

	if out, err := exec.Command("uname").Output(); err != nil {
		return nil, err
	} else {
		output["OS Type"] = strings.Trim(string(out), "\n")
	}
	//output["Physical Memory"] = make(map[string]string)
	if forcemem := OS.configuration.GetMemoryLimit(); forcemem == 0 {
		virtualMemory, _ := mem.VirtualMemory()
		output["Physical Memory"] = map[string]uint64{"bytes": uint64(virtualMemory.Total)}
	} else {
		output["Physical Memory"] = map[string]uint64{"bytes": uint64(forcemem * 1048576)}
	}

	swapMemory, _ := mem.SwapMemory()
	output["Swap Memory"] = map[string]uint64{"bytes": uint64(swapMemory.Total)}

	if is_virtual_machine(output["OS Type"].(string)) == 0 {
		output["Virtual Machine"] = "YES"
	} else {
		output["Virtual Machine"] = "NO"
	}
	output["NbCore"] = cpu_cores(output["OS Type"].(string))

	metrics := Metric{"OS": output}
	OS.logger.Debug("collectMetrics ", output)
	return metrics, nil

}
