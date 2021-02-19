package common

import (
	"errors"
	"io/ioutil"
	"os"

	"google.golang.org/grpc/grpclog"
)

var (
	logFileName = ""

	logComponent = grpclog.Component("unknown")
	// ErrUnknownSeverity ...
	ErrUnknownSeverity = errors.New("Unknown logger severity")

	// InfoSeverity ...
	InfoSeverity = "INFO"
	// WarningSeverity ...
	WarningSeverity = "WARNING"
	// ErrorSeverity ...
	ErrorSeverity = "ERROR"
)

func init() {
	_ = LoggerInit("unknown", "", InfoSeverity)
}

func getLogFile(logFile string) (*os.File, error) {
	if len(logFile) == 0 {
		return os.Stdout, nil
	}
	return os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
}

// LoggerInit ...
func LoggerInit(component string, logFile string, severity string) error {
	file, err := getLogFile(logFile)
	if err != nil {
		return err
	}

	switch severity {
	case InfoSeverity:
		grpclog.SetLoggerV2(grpclog.NewLoggerV2WithVerbosity(file, ioutil.Discard, ioutil.Discard, 2))
	case WarningSeverity:
		grpclog.SetLoggerV2(grpclog.NewLoggerV2WithVerbosity(ioutil.Discard, file, ioutil.Discard, 2))
	case ErrorSeverity:
		grpclog.SetLoggerV2(grpclog.NewLoggerV2WithVerbosity(ioutil.Discard, ioutil.Discard, file, 2))
	default:
		return ErrUnknownSeverity
	}

	logFileName = logFile
	logComponent = grpclog.Component(component)
	return nil
}

// GetLogFileName ...
func GetLogFileName() string {
	return logFileName
}

// LogInfo ...
func LogInfo(v ...interface{}) {
	logComponent.Info(v...)
}

// LogWarning ...
func LogWarning(v ...interface{}) {
	logComponent.Warning(v...)
}

// LogError ...
func LogError(v ...interface{}) {
	logComponent.Error(v...)
}

// LogFatal ...
func LogFatal(v ...interface{}) {
	logComponent.Fatal(v...)
}
