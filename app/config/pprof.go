package config

import (
	"fmt"
	"github.com/mosn/holmes"
	"github.com/spf13/viper"
	tmos "github.com/tendermint/tendermint/libs/os"
)

type PporfConfig struct {
	autoDump              bool
	collectInterval       string
	coolDown              string
	dumpPath              string
	cpuTriggerPercentMin  int
	cpuTriggerPercentDiff int
	cpuTriggerPercentAbs  int
}

const (
	FlagPprofAutoDump              = "pprof-auto-dump"
	FlagPprofCollectInterval       = "pprof-collect-interval"
	FlagPprofCoolDown              = "pprof-cool-down"
	FlagPprofDumpPath              = "pprof-dump-path"
	FlagPprofCpuTriggerPercentMin  = "pprof-cpu-trigger-percent-min"
	FlagPprofCpuTriggerPercentDiff = "pprof-cpu-trigger-percent-diff"
	FlagPprofCpuTriggerPercentAbs  = "pprof-cpu-trigger-percent-abs"
)

// PprofDown auto dump pprof
func PprofDown() {
	c := LoadPprofFromConfig()
	fmt.Println(fmt.Sprintf("LoadPprofFromConfig = %v", c))
	if c.autoDump {
		h, err := holmes.New(
			holmes.WithCollectInterval(c.collectInterval),
			holmes.WithCoolDown(c.coolDown),
			holmes.WithDumpPath(c.dumpPath),
			holmes.WithCPUDump(c.cpuTriggerPercentMin, c.cpuTriggerPercentDiff, c.cpuTriggerPercentAbs),
			holmes.WithBinaryDump(),
		)
		if err != nil {
			tmos.Exit(err.Error())
		}
		h.EnableCPUDump()
		// start the metrics collect and dump loop
		h.Start()
		fmt.Println("auto dump pprof start")
	}
}

func LoadPprofFromConfig() *PporfConfig {
	autoDump := viper.GetBool(FlagPprofAutoDump)
	collectInterval := viper.GetString(FlagPprofCollectInterval)
	coolDown := viper.GetString(FlagPprofCoolDown)
	dumpPath := viper.GetString(FlagPprofDumpPath)
	cpuTriggerPercentMin := viper.GetInt(FlagPprofCpuTriggerPercentMin)
	cpuTriggerPercentDiff := viper.GetInt(FlagPprofCpuTriggerPercentDiff)
	cpuTriggerPercentAbs := viper.GetInt(FlagPprofCpuTriggerPercentAbs)

	c := &PporfConfig{
		autoDump:              autoDump,
		collectInterval:       collectInterval,
		coolDown:              coolDown,
		dumpPath:              dumpPath,
		cpuTriggerPercentMin:  cpuTriggerPercentMin,
		cpuTriggerPercentDiff: cpuTriggerPercentDiff,
		cpuTriggerPercentAbs:  cpuTriggerPercentAbs,
	}
	return c
}
