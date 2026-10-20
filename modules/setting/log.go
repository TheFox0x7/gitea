// Copyright 2019 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package setting

import (
	"fmt"
	golog "log"
	"os"
	"path/filepath"
	"strings"

	"gitea.dev/modules/log"
	"gitea.dev/modules/slog"
	"gitea.dev/modules/util"
)

type LogGlobalConfig struct {
	RootPath string

	Mode               string
	Level              log.Level
	StacktraceLogLevel log.Level

	EnableSSHLog bool

	RequestIDHeaders []string
}

var Log LogGlobalConfig

func loadLogGlobalFrom(rootCfg ConfigProvider) {
	sec := rootCfg.Section("log")

	Log.Level = log.LevelFromString(sec.Key("LEVEL").MustString(log.INFO.String()))
	Log.StacktraceLogLevel = log.LevelFromString(sec.Key("STACKTRACE_LEVEL").MustString(log.NONE.String()))
	Log.Mode = sec.Key("MODE").MustString("console")
	Log.RootPath = sec.Key("ROOT_PATH").MustString(filepath.Join(AppWorkPath, "log"))
	if !filepath.IsAbs(Log.RootPath) {
		Log.RootPath = filepath.Join(AppWorkPath, Log.RootPath)
	}
	Log.RootPath = util.FilePathJoinAbs(Log.RootPath)

	Log.EnableSSHLog = sec.HasKey("ENABLE_SSH_LOG") && sec.Key("ENABLE_SSH_LOG").MustBool(false)

	Log.RequestIDHeaders = sec.Key("REQUEST_ID_HEADERS").Strings(",")
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
		Level:           log.LevelFromString(ConfigInheritedKeyString(sec, "LEVEL", Log.Level.String())),
		StacktraceLevel: log.LevelFromString(ConfigInheritedKeyString(sec, "STACKTRACE_LEVEL", Log.StacktraceLogLevel.String())),
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
		fileName := LogPrepareFilenameForWriter(ConfigInheritedKey(sec, "FILE_NAME").String(), defaultFileName)
		sink.File = &slog.FileSinkOptions{
			FileName:         fileName + filenameSuffix,
			LogRotate:        ConfigInheritedKey(sec, "LOG_ROTATE").MustBool(true),
			MaxSize:          1 << uint(ConfigInheritedKey(sec, "MAX_SIZE_SHIFT").MustInt(28)),
			DailyRotate:      ConfigInheritedKey(sec, "DAILY_ROTATE").MustBool(true),
			MaxDays:          ConfigInheritedKey(sec, "MAX_DAYS").MustInt(7),
			Compress:         ConfigInheritedKey(sec, "COMPRESS").MustBool(true),
			CompressionLevel: ConfigInheritedKey(sec, "COMPRESSION_LEVEL").MustInt(-1),
		}
	case "conn":
		sink.Conn = &slog.ConnSinkOptions{
			ReconnectOnMsg: ConfigInheritedKey(sec, "RECONNECT_ON_MSG").MustBool(),
			Reconnect:      ConfigInheritedKey(sec, "RECONNECT").MustBool(),
			Protocol:       ConfigInheritedKey(sec, "PROTOCOL").In("tcp", []string{"tcp", "unix", "udp"}),
			Addr:           ConfigInheritedKey(sec, "ADDR").MustString(":7020"),
		}
	}
	// other writer types come from the sink-type registry (e.g. "test") and
	// are validated by Apply
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

func initManagedLoggers(manager *slog.Manager, cfg ConfigProvider) {
	loadLogGlobalFrom(cfg)
	prepareLoggerConfig(cfg)

	config := slog.Config{}
	// the closed set of named loggers users can configure; components opt in
	// by logging through their named logger (e.g. modules/ssh → "ssh")
	for _, name := range []string{log.DEFAULT, "access", "router", "xorm", "ssh"} {
		if loggerConfig, ok := loggerConfigByName(cfg, name); ok {
			config.Loggers = append(config.Loggers, loggerConfig)
		}
	}
	if err := manager.Apply(config); err != nil {
		log.FallbackErrorf("Failed to apply log config: %v", err)
	}
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
		modeVal = Log.Mode
	}

	loggerConfig := slog.LoggerConfig{Name: loggerName}
	for modeName := range strings.SplitSeq(modeVal, ",") {
		modeName = strings.TrimSpace(modeName)
		if modeName == "" {
			continue
		}
		// BUFFER_LEN is accepted in configs but ignored: the engine writes
		// synchronously, there is no queue to size
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
