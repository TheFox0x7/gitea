// Copyright 2019 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package setting

import (
	"path/filepath"
	"testing"

	"gitea.dev/modules/json"
	"gitea.dev/modules/slog"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func toJSON(v any) string {
	b, _ := json.MarshalIndent(v, "", "\t")
	return string(b)
}

func initLoggersByConfig(t *testing.T, config string) (*slog.Manager, func()) {
	oldLogConfig := Log
	Log = LogGlobalConfig{}
	t.Cleanup(func() {
		Log = oldLogConfig
	})

	cfg, err := NewConfigProviderFromData(config)
	assert.NoError(t, err)

	manager := slog.NewManager()
	initManagedLoggers(manager, cfg)
	return manager, manager.Close
}

// loggerDumpJSON returns the sinks of a logger in the shape of DumpWriters:
// map[sinkName]sinkDetails, "{}" for a missing or sink-less logger.
func loggerDumpJSON(t *testing.T, manager *slog.Manager, name string) string {
	t.Helper()
	dump := manager.Dump()
	loggers := dump["loggers"].(map[string]any) //nolint:forcetypeassert // constructed by slog.Manager.Dump
	lm, ok := loggers[name]
	if !ok {
		return "{}"
	}
	sinkNames := lm.(map[string]any)["sinks"].([]string) //nolint:forcetypeassert // constructed by slog.Manager.Dump
	sinks := dump["sinks"].(map[string]any)              //nolint:forcetypeassert // constructed by slog.Manager.Dump
	eventWriters := map[string]any{}
	for _, sn := range sinkNames {
		eventWriters[sn] = sinks[sn]
	}
	return toJSON(eventWriters)
}

func TestLogConfigDefault(t *testing.T) {
	manager, managerClose := initLoggersByConfig(t, ``)
	defer managerClose()

	sinkDump := `
{
	"console": {
		"bufferLen": 10000,
		"colorize": false,
		"flags": "stdflags",
		"format": "legacy",
		"level": "info",
		"prefix": "",
		"stacktraceLevel": "none",
		"type": "console"
	}
}
`

	require.JSONEq(t, sinkDump, loggerDumpJSON(t, manager, "default"))
	assert.Equal(t, "{}", loggerDumpJSON(t, manager, "access"))
	require.JSONEq(t, sinkDump, loggerDumpJSON(t, manager, "router"))
	require.JSONEq(t, sinkDump, loggerDumpJSON(t, manager, "xorm"))
}

func TestLogConfigDisable(t *testing.T) {
	manager, managerClose := initLoggersByConfig(t, `
[log]
logger.router.MODE =
logger.xorm.MODE =
`)
	defer managerClose()

	sinkDump := `
{
	"console": {
		"bufferLen": 10000,
		"colorize": false,
		"flags": "stdflags",
		"format": "legacy",
		"level": "info",
		"prefix": "",
		"stacktraceLevel": "none",
		"type": "console"
	}
}
`

	require.JSONEq(t, sinkDump, loggerDumpJSON(t, manager, "default"))
	assert.Equal(t, "{}", loggerDumpJSON(t, manager, "access"))
	assert.Equal(t, "{}", loggerDumpJSON(t, manager, "router"))
	assert.Equal(t, "{}", loggerDumpJSON(t, manager, "xorm"))
}

func TestLogConfigLegacyDefault(t *testing.T) {
	manager, managerClose := initLoggersByConfig(t, `
[log]
MODE = console
`)
	defer managerClose()

	sinkDump := `
{
	"console": {
		"bufferLen": 10000,
		"colorize": false,
		"flags": "stdflags",
		"format": "legacy",
		"level": "info",
		"prefix": "",
		"stacktraceLevel": "none",
		"type": "console"
	}
}
`

	require.JSONEq(t, sinkDump, loggerDumpJSON(t, manager, "default"))
	assert.Equal(t, "{}", loggerDumpJSON(t, manager, "access"))
	require.JSONEq(t, sinkDump, loggerDumpJSON(t, manager, "router"))
	require.JSONEq(t, sinkDump, loggerDumpJSON(t, manager, "xorm"))
}

func TestLogConfigLegacyMode(t *testing.T) {
	tempDir := t.TempDir()

	tempPath := func(file string) string {
		return filepath.Join(tempDir, file)
	}
	_ = tempPath // file names are owned by the file sink writer, not exposed by Dump

	manager, managerClose := initLoggersByConfig(t, `
[log]
ROOT_PATH = `+tempDir+`
MODE = file
ROUTER = file
ACCESS = file
`)
	defer managerClose()

	sinkDump := `
{
	"file": {
		"bufferLen": 10000,
		"colorize": false,
		"flags": "stdflags",
		"format": "legacy",
		"level": "info",
		"prefix": "",
		"stacktraceLevel": "none",
		"type": "file"
	}
}
`
	sinkDumpAccess := `
{
	"file.access": {
		"bufferLen": 10000,
		"colorize": false,
		"flags": "none",
		"format": "legacy",
		"level": "info",
		"prefix": "",
		"stacktraceLevel": "none",
		"type": "file"
	}
}
`
	require.JSONEq(t, sinkDump, loggerDumpJSON(t, manager, "default"))
	require.JSONEq(t, sinkDumpAccess, loggerDumpJSON(t, manager, "access"))
	require.JSONEq(t, sinkDump, loggerDumpJSON(t, manager, "router"))
	require.JSONEq(t, sinkDump, loggerDumpJSON(t, manager, "xorm"))
}

func TestLogConfigLegacyModeDisable(t *testing.T) {
	manager, managerClose := initLoggersByConfig(t, `
[log]
ROUTER = file
ACCESS = file
DISABLE_ROUTER_LOG = true
ENABLE_ACCESS_LOG = false
`)
	defer managerClose()

	assert.Equal(t, "{}", loggerDumpJSON(t, manager, "access"))
	assert.Equal(t, "{}", loggerDumpJSON(t, manager, "router"))
}

func TestLogConfigNewConfig(t *testing.T) {
	manager, managerClose := initLoggersByConfig(t, `
[log]
logger.access.MODE = console
logger.xorm.MODE = console, console-1

[log.console]
LEVEL = warn

[log.console-1]
MODE = console
LEVEL = error
FORMAT = logfmt
STDERR = true
`)
	defer managerClose()

	sinkDump := `
{
	"console": {
		"bufferLen": 10000,
		"colorize": false,
		"flags": "stdflags",
		"format": "legacy",
		"level": "warn",
		"prefix": "",
		"stacktraceLevel": "none",
		"type": "console"
	},
	"console-1": {
		"bufferLen": 10000,
		"colorize": false,
		"flags": "stdflags",
		"format": "logfmt",
		"level": "error",
		"prefix": "",
		"stacktraceLevel": "none",
		"type": "console"
	}
}
`
	sinkDumpAccess := `
{
	"console.access": {
		"bufferLen": 10000,
		"colorize": false,
		"flags": "none",
		"format": "legacy",
		"level": "warn",
		"prefix": "",
		"stacktraceLevel": "none",
		"type": "console"
	}
}
`
	require.JSONEq(t, sinkDump, loggerDumpJSON(t, manager, "xorm"))
	require.JSONEq(t, sinkDumpAccess, loggerDumpJSON(t, manager, "access"))
}

func TestLogConfigModeFile(t *testing.T) {
	tempDir := t.TempDir()

	manager, managerClose := initLoggersByConfig(t, `
[log]
ROOT_PATH = `+tempDir+`
BUFFER_LEN = 10
MODE = file, file1

[log.file1]
MODE = file
LEVEL = error
STACKTRACE_LEVEL = fatal
FLAGS = medfile
PREFIX = "[Prefix] "
FILE_NAME = file-xxx.log
LOG_ROTATE = false
MAX_SIZE_SHIFT = 1
DAILY_ROTATE = false
MAX_DAYS = 90
COMPRESS = false
COMPRESSION_LEVEL = 4
`)
	defer managerClose()

	sinkDump := `
{
	"file": {
		"bufferLen": 10,
		"colorize": false,
		"flags": "stdflags",
		"format": "legacy",
		"level": "info",
		"prefix": "",
		"stacktraceLevel": "none",
		"type": "file"
	},
	"file1": {
		"bufferLen": 10,
		"colorize": false,
		"flags": "medfile",
		"format": "legacy",
		"level": "error",
		"prefix": "[Prefix] ",
		"stacktraceLevel": "fatal",
		"type": "file"
	}
}
`
	require.JSONEq(t, sinkDump, loggerDumpJSON(t, manager, "default"))
}
