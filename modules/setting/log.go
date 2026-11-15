// Copyright 2019 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package setting

import (
	"fmt"
	golog "log"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"gitea.dev/modules/log"
	"gitea.dev/modules/slog"
	"gitea.dev/modules/util"
)

type LogGlobalConfig struct {
	RootPath string

	Level              log.Level
	StacktraceLogLevel log.Level

	EnableSSHLog bool

	RequestIDHeaders []string
}

var Log LogGlobalConfig

func loadLogGlobalFrom(rootCfg ConfigProvider) {
	sec := rootCfg.Section("log")

	Log.Level = log.LevelFromString(sec.Key("LEVEL").MustString(log.LevelName(log.INFO)))
	Log.StacktraceLogLevel = log.LevelFromString(sec.Key("STACKTRACE_LEVEL").MustString(log.LevelName(log.NONE)))
	Log.RootPath = sec.Key("ROOT_PATH").MustString(filepath.Join(AppWorkPath, "log"))
	if !filepath.IsAbs(Log.RootPath) {
		Log.RootPath = filepath.Join(AppWorkPath, Log.RootPath)
	}
	Log.RootPath = util.FilePathJoinAbs(Log.RootPath)

	Log.EnableSSHLog = sec.HasKey("ENABLE_SSH_LOG") && sec.Key("ENABLE_SSH_LOG").MustBool(false)

	Log.RequestIDHeaders = sec.Key("REQUEST_ID_HEADERS").Strings(",")
}

// logComponentLoggers is the closed set of named loggers users can configure;
// components opt in by logging through their named logger (e.g. modules/ssh → "ssh").
var logComponentLoggers = []string{"access", "router", "xorm", "ssh"}

const (
	logSinksPrefix    = "log.sinks."
	logLoggersSection = "log.loggers"
)

// defaultLogMaxSize is the historical MAX_SIZE_SHIFT=28 default.
const defaultLogMaxSize = "256MiB"

// builtinSinks are sink names that can be referenced in [log.loggers] without
// a [log.sinks.<name>] section.
var builtinSinks = map[string]slog.SinkConfig{
	"discard": {Name: "discard", Type: slog.SinkTypeDiscard},
}

func initManagedLoggers(manager *slog.Manager, cfg ConfigProvider) {
	loadLogGlobalFrom(cfg)

	config, err := loadLogSlogConfig(cfg)
	if err == nil {
		err = manager.Apply(config)
	}
	if err != nil {
		log.FallbackErrorf("Failed to apply log config: %v", err)
	}
}

// loadLogSlogConfig builds the slog configuration. The current schema defines
// named sinks in [log.sinks.<name>] sections and wires loggers to them in
// [log.loggers]; the historical MODE-based schema is still accepted and
// translated (see loadLogConfigLegacy).
func loadLogSlogConfig(rootCfg ConfigProvider) (slog.Config, error) {
	if hasNewLogConfig(rootCfg) {
		return loadLogConfigNew(rootCfg)
	}
	return loadLogConfigLegacy(rootCfg), nil
}

func hasNewLogConfig(rootCfg ConfigProvider) bool {
	if sec, err := rootCfg.GetSection(logLoggersSection); err == nil && len(sec.Keys()) > 0 {
		return true
	}
	for _, sec := range rootCfg.Sections() {
		if strings.HasPrefix(sec.Name(), logSinksPrefix) {
			return true
		}
	}
	return false
}

// loadLogConfigNew reads the structured schema:
//
//	[log.sinks.<name>]   one section per sink, TYPE selects console/file/conn
//	[log.loggers]        maps a logger name to a comma-separated list of sink names
//
// A logger without an entry follows "default" (except "access", which stays
// disabled unless wired); an empty list disables the logger.
func loadLogConfigNew(rootCfg ConfigProvider) (slog.Config, error) {
	sinks := map[string]slog.SinkConfig{}
	for _, sec := range rootCfg.Sections() {
		name := sec.Name()
		if !strings.HasPrefix(name, logSinksPrefix) {
			continue
		}
		sinkName := strings.TrimPrefix(name, logSinksPrefix)
		if sinkName == "" {
			continue
		}
		sc, err := loadSinkConfig(sec, sinkName)
		if err != nil {
			return slog.Config{}, err
		}
		sinks[sinkName] = sc
	}

	wiring := map[string][]string{}
	if sec, err := rootCfg.GetSection(logLoggersSection); err == nil {
		for _, k := range sec.Keys() {
			var names []string
			for _, n := range k.Strings(",") {
				if n = strings.TrimSpace(n); n != "" {
					names = append(names, n)
				}
			}
			wiring[strings.ToLower(k.Name())] = names
		}
	}
	if _, ok := wiring["default"]; !ok {
		// no explicit wiring: the default logger gets every defined sink
		names := make([]string, 0, len(sinks))
		for name := range sinks {
			names = append(names, name)
		}
		slices.Sort(names)
		wiring["default"] = names
	}
	for _, name := range logComponentLoggers {
		if name == "access" {
			continue // access stays disabled unless explicitly wired
		}
		if _, ok := wiring[name]; !ok {
			wiring[name] = wiring["default"]
		}
	}
	// the ssh component follows the default logger unless ENABLE_SSH_LOG is
	// explicitly false (same semantics as the legacy schema)
	if logSec := rootCfg.Section("log"); logSec.HasKey("ENABLE_SSH_LOG") && !logSec.Key("ENABLE_SSH_LOG").MustBool() {
		wiring["ssh"] = nil
	}

	// any logger named in [log.loggers] is configurable, not just the known
	// components: a new component can be tuned without a code change
	names := map[string]bool{slog.DefaultName: true}
	for _, n := range logComponentLoggers {
		names[n] = true
	}
	for n := range wiring {
		names[n] = true
	}

	config := slog.Config{}
	for _, name := range slices.Sorted(maps.Keys(names)) {
		sinkNames := wiring[name]
		if len(sinkNames) == 0 {
			continue // disabled
		}
		lc := slog.LoggerConfig{Name: name}
		for _, sn := range sinkNames {
			sc, ok := sinks[sn]
			if !ok {
				sc, ok = builtinSinks[sn]
			}
			if !ok {
				return slog.Config{}, fmt.Errorf("log logger %q references undefined sink %q", name, sn)
			}
			lc.Sinks = append(lc.Sinks, sc)
		}
		config.Loggers = append(config.Loggers, lc)
	}
	return config, nil
}

// loadSinkConfig reads one [log.sinks.<name>] section. Fields not set in the
// section fall back to the [log] globals (LEVEL, STACKTRACE_LEVEL).
func loadSinkConfig(sec ConfigSection, name string) (slog.SinkConfig, error) {
	sinkType := strings.ToLower(ConfigSectionKeyString(sec, "TYPE"))
	switch sinkType {
	case "console", "file", "conn", slog.SinkTypeDiscard:
	default:
		return slog.SinkConfig{}, fmt.Errorf("log sink %q has invalid TYPE %q (want console, file, conn or discard)", name, sinkType)
	}

	sink := slog.SinkConfig{
		Name:            name,
		Type:            sinkType,
		Format:          slog.Format(ConfigInheritedKeyString(sec, "FORMAT", string(slog.FormatLegacy))),
		Level:           log.LevelFromString(ConfigInheritedKeyString(sec, "LEVEL", log.LevelName(Log.Level))),
		StacktraceLevel: log.LevelFromString(ConfigInheritedKeyString(sec, "STACKTRACE_LEVEL", log.LevelName(Log.StacktraceLogLevel))),
		Prefix:          ConfigInheritedKeyString(sec, "PREFIX"),
		Flags:           slog.FlagsFromString(ConfigInheritedKeyString(sec, "FLAGS", "stdflags")),
		AddSource:       ConfigInheritedKey(sec, "WITH_CALLER").MustBool(false),
	}

	switch sinkType {
	case "console":
		useStderr := ConfigInheritedKey(sec, "STDERR").MustBool(false)
		defaultColorize := slog.CanColorStdout
		if useStderr {
			defaultColorize = slog.CanColorStderr
		}
		sink.Colorize = ConfigInheritedKey(sec, "COLORIZE").MustBool(defaultColorize)
		sink.Console = &slog.ConsoleSinkOptions{Stderr: useStderr}
	case "file":
		fileName := LogPrepareFilenameForWriter(ConfigInheritedKeyString(sec, "FILE_NAME"), "gitea.log")
		maxSize, err := util.ParseByteSize(ConfigInheritedKeyString(sec, "MAX_SIZE", defaultLogMaxSize))
		if err != nil {
			return slog.SinkConfig{}, fmt.Errorf("log sink %q has invalid MAX_SIZE: %v", name, err)
		}
		sink.File = &slog.FileSinkOptions{
			FileName:         fileName + filenameSuffix,
			MaxSize:          int64(maxSize),
			LogRotate:        ConfigInheritedKey(sec, "LOG_ROTATE").MustBool(true),
			DailyRotate:      ConfigInheritedKey(sec, "DAILY_ROTATE").MustBool(true),
			MaxDays:          ConfigInheritedKey(sec, "MAX_DAYS").MustInt(7),
			Compress:         ConfigInheritedKey(sec, "COMPRESS").MustBool(true),
			CompressionLevel: ConfigInheritedKey(sec, "COMPRESSION_LEVEL").MustInt(-1),
		}
	case "conn":
		sink.Conn = &slog.ConnSinkOptions{
			Protocol:     ConfigInheritedKey(sec, "PROTOCOL").In("tcp", []string{"tcp", "unix", "udp"}),
			Addr:         ConfigInheritedKey(sec, "ADDR").MustString(":7020"),
			DialTimeout:  ConfigInheritedKey(sec, "DIAL_TIMEOUT").MustDuration(30 * time.Second),
			WriteTimeout: ConfigInheritedKey(sec, "WRITE_TIMEOUT").MustDuration(60 * time.Second),
		}
	}
	return sink, nil
}

func LogPrepareFilenameForWriter(fileName, defaultFileName string) string {
	if fileName == "" {
		fileName = defaultFileName
	}
	if !filepath.IsAbs(fileName) {
		fileName = filepath.Join(Log.RootPath, fileName)
	} else {
		fileName = filepath.Clean(fileName)
	}
	if err := os.MkdirAll(filepath.Dir(fileName), os.ModePerm); err != nil {
		panic(fmt.Sprintf("unable to create directory for log %q: %v", fileName, err.Error()))
	}
	return fileName
}

// loadLogConfigLegacy translates the historical MODE-based schema
// ([log] MODE, [log.<mode>] sections, logger.<name>.MODE keys) into the slog
// configuration. Deprecated since the structured [log.sinks.*]/[log.loggers]
// schema.
func loadLogConfigLegacy(rootCfg ConfigProvider) slog.Config {
	prepareLoggerConfig(rootCfg)

	config := slog.Config{}
	for _, name := range append([]string{log.DEFAULT}, logComponentLoggers...) {
		if loggerConfig, ok := loggerConfigByName(rootCfg, name); ok {
			config.Loggers = append(config.Loggers, loggerConfig)
		}
	}
	return config
}

func prepareLoggerConfig(rootCfg ConfigProvider) {
	sec := rootCfg.Section("log")

	if !sec.HasKey("logger.default.MODE") {
		sec.Key("logger.default.MODE").MustString(",") // FIXME: INI-MUST-SIDE-EFFECT
	}

	deprecatedSetting(rootCfg, "log", "ACCESS", "log", "logger.access.MODE", "1.21")
	deprecatedSetting(rootCfg, "log", "ENABLE_ACCESS_LOG", "log", "logger.access.MODE", "1.21")
	if val := sec.Key("ACCESS").String(); val != "" {
		sec.Key("logger.access.MODE").MustString(val) // FIXME: INI-MUST-SIDE-EFFECT
	}
	if sec.HasKey("ENABLE_ACCESS_LOG") && !sec.Key("ENABLE_ACCESS_LOG").MustBool() {
		sec.Key("logger.access.MODE").SetValue("")
	}

	deprecatedSetting(rootCfg, "log", "ROUTER", "log", "logger.router.MODE", "1.21")
	deprecatedSetting(rootCfg, "log", "DISABLE_ROUTER_LOG", "log", "logger.router.MODE", "1.21")
	if val := sec.Key("ROUTER").String(); val != "" {
		sec.Key("logger.router.MODE").MustString(val) // FIXME: INI-MUST-SIDE-EFFECT
	}
	if !sec.HasKey("logger.router.MODE") {
		// Use the default logger.
		sec.Key("logger.router.MODE").MustString(",") // FIXME: INI-MUST-SIDE-EFFECT
	}
	if sec.HasKey("DISABLE_ROUTER_LOG") && sec.Key("DISABLE_ROUTER_LOG").MustBool() {
		sec.Key("logger.router.MODE").SetValue("")
	}

	deprecatedSetting(rootCfg, "log", "XORM", "log", "logger.xorm.MODE", "1.21")
	deprecatedSetting(rootCfg, "log", "ENABLE_XORM_LOG", "log", "logger.xorm.MODE", "1.21")
	if val := sec.Key("XORM").String(); val != "" {
		sec.Key("logger.xorm.MODE").MustString(val) // FIXME: INI-MUST-SIDE-EFFECT
	}
	if !sec.HasKey("logger.xorm.MODE") {
		// Use the default logger.
		sec.Key("logger.xorm.MODE").MustString(",") // FIXME: INI-MUST-SIDE-EFFECT
	}
	if sec.HasKey("ENABLE_XORM_LOG") && !sec.Key("ENABLE_XORM_LOG").MustBool() {
		sec.Key("logger.xorm.MODE").SetValue("")
	}

	// the ssh component follows the default logger unless explicitly configured;
	// an explicit ENABLE_SSH_LOG = false disables it (the historical setting was
	// dead code before)
	switch {
	case sec.HasKey("ENABLE_SSH_LOG") && !sec.Key("ENABLE_SSH_LOG").MustBool():
		sec.Key("logger.ssh.MODE").SetValue("")
	case !sec.HasKey("logger.ssh.MODE"):
		sec.Key("logger.ssh.MODE").SetValue(sec.Key("logger.default.MODE").String())
	}
}

// loadLogModeByName builds one sink from a legacy [log.<mode>] section.
func loadLogModeByName(rootCfg ConfigProvider, loggerName, modeName string) slog.SinkConfig {
	sec := rootCfg.Section("log." + modeName)

	writerType := ConfigSectionKeyString(sec, "MODE")
	if writerType == "" {
		writerType = modeName
	}

	writerName := modeName
	defaultFlags := "stdflags"
	defaultFileName := "gitea.log"
	defaultFormat := slog.FormatLegacy
	if loggerName == "access" {
		// "access" logger is special, by default it doesn't have output flags, so it also needs a new writer name to avoid conflicting with other writers.
		// so "access" logger's writer name is usually "file.access" or "console.access"
		writerName += ".access"
		defaultFlags = "none"
		defaultFileName = "access.log"
		defaultFormat = slog.FormatCLF // access records carry structured request fields; the sink renders the combined log format
	}

	sink := slog.SinkConfig{
		Name:            writerName,
		Type:            writerType,
		Level:           log.LevelFromString(ConfigInheritedKeyString(sec, "LEVEL", log.LevelName(Log.Level))),
		StacktraceLevel: log.LevelFromString(ConfigInheritedKeyString(sec, "STACKTRACE_LEVEL", log.LevelName(Log.StacktraceLogLevel))),
		Prefix:          ConfigInheritedKeyString(sec, "PREFIX"),
		Flags:           slog.FlagsFromString(ConfigInheritedKeyString(sec, "FLAGS", defaultFlags)),
		Format:          slog.Format(ConfigInheritedKeyString(sec, "FORMAT", string(defaultFormat))),
		AddSource:       ConfigInheritedKey(sec, "WITH_CALLER").MustBool(false),
	}

	switch writerType {
	case "console":
		useStderr := ConfigInheritedKey(sec, "STDERR").MustBool(false)
		defaultCanColor := slog.CanColorStdout
		if useStderr {
			defaultCanColor = slog.CanColorStderr
		}
		sink.Colorize = ConfigInheritedKey(sec, "COLORIZE").MustBool(defaultCanColor)
		sink.Console = &slog.ConsoleSinkOptions{Stderr: useStderr}
	case "file":
		fileName := LogPrepareFilenameForWriter(ConfigInheritedKeyString(sec, "FILE_NAME"), defaultFileName)
		maxSize := int64(1 << uint(ConfigInheritedKey(sec, "MAX_SIZE_SHIFT").MustInt(28))) // deprecated: use MAX_SIZE in [log.sinks.*]
		if v := ConfigInheritedKeyString(sec, "MAX_SIZE"); v != "" {
			if parsed, err := util.ParseByteSize(v); err == nil {
				maxSize = int64(parsed)
			}
		}
		sink.File = &slog.FileSinkOptions{
			FileName:         fileName + filenameSuffix,
			MaxSize:          maxSize,
			LogRotate:        ConfigInheritedKey(sec, "LOG_ROTATE").MustBool(true),
			DailyRotate:      ConfigInheritedKey(sec, "DAILY_ROTATE").MustBool(true),
			MaxDays:          ConfigInheritedKey(sec, "MAX_DAYS").MustInt(7),
			Compress:         ConfigInheritedKey(sec, "COMPRESS").MustBool(true),
			CompressionLevel: ConfigInheritedKey(sec, "COMPRESSION_LEVEL").MustInt(-1),
		}
	case "conn":
		sink.Conn = &slog.ConnSinkOptions{
			Protocol:     ConfigInheritedKey(sec, "PROTOCOL").In("tcp", []string{"tcp", "unix", "udp"}),
			Addr:         ConfigInheritedKey(sec, "ADDR").MustString(":7020"),
			DialTimeout:  ConfigInheritedKey(sec, "DIAL_TIMEOUT").MustDuration(30 * time.Second),
			WriteTimeout: ConfigInheritedKey(sec, "WRITE_TIMEOUT").MustDuration(60 * time.Second),
		}
	}
	// BUFFER_LEN is accepted in legacy configs but ignored: the engine writes
	// synchronously, there is no queue to size
	return sink
}

var filenameSuffix = ""

// RestartLogsWithPIDSuffix restarts the logs with a PID suffix on files
// FIXME: it seems not right, it breaks log rotating or log collectors
func RestartLogsWithPIDSuffix() {
	filenameSuffix = fmt.Sprintf(".%d", os.Getpid())
	initAllLoggers() // when forking, before restarting, rename logger file and re-init all loggers
}

func InitLoggersForTest() {
	initAllLoggers()
}

var initLoggerDisabled bool

// initAllLoggers creates all the log services
func initAllLoggers() {
	if initLoggerDisabled {
		return
	}
	initManagedLoggers(slog.GetManager(), CfgProvider)

	golog.SetFlags(0)
	golog.SetPrefix("")
	golog.SetOutput(gologBridge{
		handler: slog.GetLogger(slog.DefaultName).Handler(),
		level:   slog.LevelInfo,
	})
}

func DisableLoggerInit() {
	initLoggerDisabled = true
}

func loggerConfigByName(rootCfg ConfigProvider, loggerName string) (slog.LoggerConfig, bool) {
	sec := rootCfg.Section("log")
	keyPrefix := "logger." + loggerName

	disabled := sec.HasKey(keyPrefix+".MODE") && sec.Key(keyPrefix+".MODE").String() == ""
	if disabled {
		return slog.LoggerConfig{}, false
	}

	modeVal := sec.Key(keyPrefix + ".MODE").String()
	if modeVal == "," {
		modeVal = ConfigSectionKeyString(sec, "MODE", "console")
	}

	loggerConfig := slog.LoggerConfig{Name: loggerName}
	for modeName := range strings.SplitSeq(modeVal, ",") {
		modeName = strings.TrimSpace(modeName)
		if modeName == "" {
			continue
		}
		loggerConfig.Sinks = append(loggerConfig.Sinks, loadLogModeByName(rootCfg, loggerName, modeName))
	}
	return loggerConfig, true
}

func InitSQLLoggersForCli(level log.Level) {
	log.SetupStderrLogger("xorm", "console-stderr", level)
}

func IsAccessLogEnabled() bool {
	return log.IsLoggerEnabled("access")
}

func IsRouteLogEnabled() bool {
	return log.IsLoggerEnabled("router")
}
