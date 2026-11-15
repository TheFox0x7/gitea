// Copyright 2019 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package setting

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

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

func loadLogSlogConfigFromData(t *testing.T, config string) (slog.Config, error) {
	t.Helper()
	oldLogConfig := Log
	Log = LogGlobalConfig{}
	t.Cleanup(func() { Log = oldLogConfig })

	cfg, err := NewConfigProviderFromData(config)
	require.NoError(t, err)
	loadLogGlobalFrom(cfg)
	return loadLogSlogConfig(cfg)
}

func loggerSinkNames(c slog.Config, name string) []string {
	for _, lc := range c.Loggers {
		if lc.Name == name {
			names := make([]string, 0, len(lc.Sinks))
			for _, sc := range lc.Sinks {
				names = append(names, sc.Name)
			}
			return names
		}
	}
	return nil
}

func configSinks(c slog.Config) map[string]slog.SinkConfig {
	sinks := map[string]slog.SinkConfig{}
	for _, lc := range c.Loggers {
		for _, sc := range lc.Sinks {
			sinks[sc.Name] = sc
		}
	}
	return sinks
}

func TestLogConfigSinksDefault(t *testing.T) {
	c, err := loadLogSlogConfigFromData(t, `
[log.sinks.console]
TYPE = console
`)
	require.NoError(t, err)
	assert.Equal(t, []string{"console"}, loggerSinkNames(c, "default"))
	assert.Equal(t, []string{"console"}, loggerSinkNames(c, "router"))
	assert.Equal(t, []string{"console"}, loggerSinkNames(c, "xorm"))
	assert.Equal(t, []string{"console"}, loggerSinkNames(c, "ssh"))
	assert.Nil(t, loggerSinkNames(c, "access")) // disabled unless wired

	sc := configSinks(c)["console"]
	assert.Equal(t, "console", sc.Type)
	assert.Equal(t, slog.FormatLegacy, sc.Format)
	assert.Equal(t, slog.LevelInfo, sc.Level)
	assert.Equal(t, slog.LevelNone, sc.StacktraceLevel)
	assert.NotNil(t, sc.Console)
	assert.False(t, sc.Console.Stderr)
}

func TestLogConfigSinksWiring(t *testing.T) {
	tempDir := t.TempDir()

	c, err := loadLogSlogConfigFromData(t, `
[log]
ROOT_PATH = `+tempDir+`
LEVEL = warn
ENABLE_SSH_LOG = false

[log.sinks.console]
TYPE = console
STDERR = true
FORMAT = json
WITH_CALLER = true

[log.sinks.logs]
TYPE = file
FILE_NAME = gitea.log
MAX_SIZE = 1GiB
LEVEL = debug

[log.sinks.access]
TYPE = file
FILE_NAME = access.log
FORMAT = clf
FLAGS = none

[log.loggers]
default = console, logs
access = access
router =
`)
	require.NoError(t, err)
	assert.Equal(t, []string{"console", "logs"}, loggerSinkNames(c, "default"))
	assert.Equal(t, []string{"access"}, loggerSinkNames(c, "access"))
	assert.Nil(t, loggerSinkNames(c, "router"))                              // disabled
	assert.Equal(t, []string{"console", "logs"}, loggerSinkNames(c, "xorm")) // follows default
	assert.Nil(t, loggerSinkNames(c, "ssh"))                                 // ENABLE_SSH_LOG = false

	sinks := configSinks(c)
	console := sinks["console"]
	assert.Equal(t, slog.FormatJSON, console.Format)
	assert.Equal(t, slog.LevelWarn, console.Level) // inherits [log] LEVEL
	assert.True(t, console.AddSource)
	assert.True(t, console.Console.Stderr)

	file := sinks["logs"]
	assert.Equal(t, int64(1<<30), file.File.MaxSize)
	assert.Equal(t, slog.LevelDebug, file.Level)
	assert.Equal(t, filepath.Join(tempDir, "gitea.log"), file.File.FileName)

	access := sinks["access"]
	assert.Equal(t, slog.FormatCLF, access.Format)
	assert.Equal(t, uint32(0), access.Flags.Bits()) // "none"
	assert.Equal(t, filepath.Join(tempDir, "access.log"), access.File.FileName)
}

func TestLogConfigSinksConn(t *testing.T) {
	c, err := loadLogSlogConfigFromData(t, `
[log.sinks.remote]
TYPE = conn
ADDR = example.com:7020
PROTOCOL = udp
DIAL_TIMEOUT = 5s
`)
	require.NoError(t, err)
	sc := configSinks(c)["remote"]
	require.NotNil(t, sc.Conn)
	assert.Equal(t, "udp", sc.Conn.Protocol)
	assert.Equal(t, "example.com:7020", sc.Conn.Addr)
	assert.Equal(t, 5*time.Second, sc.Conn.DialTimeout)
	assert.Equal(t, 60*time.Second, sc.Conn.WriteTimeout) // default
}

func TestLogConfigSinksErrors(t *testing.T) {
	// wiring without any sink definition
	_, err := loadLogSlogConfigFromData(t, `
[log.loggers]
default = console
`)
	require.Error(t, err)

	// reference to an undefined sink
	_, err = loadLogSlogConfigFromData(t, `
[log.sinks.console]
TYPE = console

[log.loggers]
default = console, nope
`)
	require.ErrorContains(t, err, "undefined sink")

	// invalid sink type
	_, err = loadLogSlogConfigFromData(t, `
[log.sinks.bad]
TYPE = carrier-pigeon
`)
	require.ErrorContains(t, err, "invalid TYPE")

	// invalid MAX_SIZE
	_, err = loadLogSlogConfigFromData(t, `
[log.sinks.file]
TYPE = file
MAX_SIZE = lots
`)
	require.ErrorContains(t, err, "invalid MAX_SIZE")
}

func TestLogConfigSinksArbitraryLogger(t *testing.T) {
	// any logger name can be wired, not just the known components
	c, err := loadLogSlogConfigFromData(t, `
[log.sinks.console]
TYPE = console

[log.loggers]
git = console
router =
`)
	require.NoError(t, err)
	assert.Equal(t, []string{"console"}, loggerSinkNames(c, "git"))
	assert.Nil(t, loggerSinkNames(c, "router"))
	assert.Equal(t, []string{"console"}, loggerSinkNames(c, "default")) // all sinks
}

func TestLogConfigSinksDiscard(t *testing.T) {
	// the built-in "discard" sink needs no [log.sinks.discard] section
	c, err := loadLogSlogConfigFromData(t, `
[log.sinks.console]
TYPE = console

[log.loggers]
default = console
router = discard
`)
	require.NoError(t, err)
	assert.Equal(t, []string{"console"}, loggerSinkNames(c, "default"))
	router := loggerSinkNames(c, "router")
	require.Equal(t, []string{"discard"}, router)
	sinks := configSinks(c)
	assert.Equal(t, slog.SinkTypeDiscard, sinks["discard"].Type)

	// and as an explicit sink type
	c, err = loadLogSlogConfigFromData(t, `
[log.sinks.void]
TYPE = discard

[log.loggers]
default = void
`)
	require.NoError(t, err)
	assert.Equal(t, slog.SinkTypeDiscard, configSinks(c)["void"].Type)
}
