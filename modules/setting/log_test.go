// Copyright 2019 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package setting

import (
	"fmt"
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
		"colorize": false,
		"addSource": false,
		"flags": "stdflags",
		"format": "legacy",
		"level": "info",
		"prefix": "",
		"stacktraceLevel": "none",
		"type": "console",
		"writer": "std:out"
	}
}
`

	require.JSONEq(t, sinkDump, loggerDumpJSON(t, manager, "default"))
	assert.Equal(t, "{}", loggerDumpJSON(t, manager, "access"))
	require.JSONEq(t, sinkDump, loggerDumpJSON(t, manager, "router"))
	require.JSONEq(t, sinkDump, loggerDumpJSON(t, manager, "xorm"))
	require.JSONEq(t, sinkDump, loggerDumpJSON(t, manager, "ssh"))
}

func TestLogConfigDisable(t *testing.T) {
	manager, managerClose := initLoggersByConfig(t, `
[log]
logger.router.MODE =
logger.xorm.MODE =
logger.ssh.MODE =
`)
	defer managerClose()

	sinkDump := `
{
	"console": {
		"colorize": false,
		"addSource": false,
		"flags": "stdflags",
		"format": "legacy",
		"level": "info",
		"prefix": "",
		"stacktraceLevel": "none",
		"type": "console",
		"writer": "std:out"
	}
}
`

	require.JSONEq(t, sinkDump, loggerDumpJSON(t, manager, "default"))
	assert.Equal(t, "{}", loggerDumpJSON(t, manager, "access"))
	assert.Equal(t, "{}", loggerDumpJSON(t, manager, "router"))
	assert.Equal(t, "{}", loggerDumpJSON(t, manager, "xorm"))
	assert.Equal(t, "{}", loggerDumpJSON(t, manager, "ssh"))
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
		"colorize": false,
		"addSource": false,
		"flags": "stdflags",
		"format": "legacy",
		"level": "info",
		"prefix": "",
		"stacktraceLevel": "none",
		"type": "console",
		"writer": "std:out"
	}
}
`

	require.JSONEq(t, sinkDump, loggerDumpJSON(t, manager, "default"))
	assert.Equal(t, "{}", loggerDumpJSON(t, manager, "access"))
	require.JSONEq(t, sinkDump, loggerDumpJSON(t, manager, "router"))
	require.JSONEq(t, sinkDump, loggerDumpJSON(t, manager, "xorm"))
	require.JSONEq(t, sinkDump, loggerDumpJSON(t, manager, "ssh"))
}

func TestLogConfigLegacyMode(t *testing.T) {
	tempDir := t.TempDir()

	// file names are owned by the file sink writer; Dump exposes them via the writer key
	writerKey := func(name string) string { return "file:" + filepath.Join(tempDir, name) }

	manager, managerClose := initLoggersByConfig(t, `
[log]
ROOT_PATH = `+tempDir+`
MODE = file
ROUTER = file
ACCESS = file
`)
	defer managerClose()

	sinkDump := fmt.Sprintf(`
{
	"file": {
		"colorize": false,
		"addSource": false,
		"flags": "stdflags",
		"format": "legacy",
		"level": "info",
		"prefix": "",
		"stacktraceLevel": "none",
		"type": "file",
		"writer": %q
	}
}
`, writerKey("gitea.log"))
	sinkDumpAccess := fmt.Sprintf(`
{
	"file.access": {
		"colorize": false,
		"addSource": false,
		"flags": "none",
		"format": "clf",
		"level": "info",
		"prefix": "",
		"stacktraceLevel": "none",
		"type": "file",
		"writer": %q
	}
}
`, writerKey("access.log"))
	require.JSONEq(t, sinkDump, loggerDumpJSON(t, manager, "default"))
	require.JSONEq(t, sinkDumpAccess, loggerDumpJSON(t, manager, "access"))
	require.JSONEq(t, sinkDump, loggerDumpJSON(t, manager, "router"))
	require.JSONEq(t, sinkDump, loggerDumpJSON(t, manager, "xorm"))
	require.JSONEq(t, sinkDump, loggerDumpJSON(t, manager, "ssh"))
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
		"colorize": false,
		"addSource": false,
		"flags": "stdflags",
		"format": "legacy",
		"level": "warn",
		"prefix": "",
		"stacktraceLevel": "none",
		"type": "console",
		"writer": "std:out"
	},
	"console-1": {
		"colorize": false,
		"addSource": false,
		"flags": "stdflags",
		"format": "logfmt",
		"level": "error",
		"prefix": "",
		"stacktraceLevel": "none",
		"type": "console",
		"writer": "std:err"
	}
}
`
	sinkDumpAccess := `
{
	"console.access": {
		"colorize": false,
		"addSource": false,
		"flags": "none",
		"format": "clf",
		"level": "warn",
		"prefix": "",
		"stacktraceLevel": "none",
		"type": "console",
		"writer": "std:out"
	}
}
`
	require.JSONEq(t, sinkDump, loggerDumpJSON(t, manager, "xorm"))
	// ssh follows the default logger (console @ warn), not xorm's sinks
	require.JSONEq(t, `{"console": {
		"colorize": false,
		"addSource": false,
		"flags": "stdflags",
		"format": "legacy",
		"level": "warn",
		"prefix": "",
		"stacktraceLevel": "none",
		"type": "console",
		"writer": "std:out"
	}}`, loggerDumpJSON(t, manager, "ssh"))
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

	sinkDump := fmt.Sprintf(`
{
	"file": {
		"colorize": false,
		"addSource": false,
		"flags": "stdflags",
		"format": "legacy",
		"level": "info",
		"prefix": "",
		"stacktraceLevel": "none",
		"type": "file",
		"writer": %q
	},
	"file1": {
		"colorize": false,
		"addSource": false,
		"flags": "medfile",
		"format": "legacy",
		"level": "error",
		"prefix": "[Prefix] ",
		"stacktraceLevel": "fatal",
		"type": "file",
		"writer": %q
	}
}
`, "file:"+filepath.Join(tempDir, "gitea.log"), "file:"+filepath.Join(tempDir, "file-xxx.log"))
	require.JSONEq(t, sinkDump, loggerDumpJSON(t, manager, "default"))
}

func TestLogConfigCallerStacktrace(t *testing.T) {
	manager, managerClose := initLoggersByConfig(t, `
[log]
logger.xorm.MODE = console

[log.console]
WITH_CALLER = true
STACKTRACE_LEVEL = error
`)
	defer managerClose()

	check := loggerDumpJSON(t, manager, "xorm")
	assert.Contains(t, check, `"addSource": true`)
	assert.Contains(t, check, `"stacktraceLevel": "error"`)
}
