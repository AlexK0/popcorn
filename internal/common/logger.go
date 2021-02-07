package common

import (
	"errors"
	"io/ioutil"
	"os"

	"google.golang.org/grpc/grpclog"
)

var (
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
	_ = LoggerInit("unknown", "", InfoSeverity, 2)
}

func getLogFile(logFile string) (*os.File, error) {
	if len(logFile) == 0 {
		return os.Stdout, nil
	}
	return os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
}

// LoggerInit ...
func LoggerInit(component string, logFile string, severity string, verbosityLevel int) error {
	file, err := getLogFile(logFile)
	if err != nil {
		return err
	}

	switch severity {
	case InfoSeverity:
		grpclog.SetLoggerV2(grpclog.NewLoggerV2WithVerbosity(file, ioutil.Discard, ioutil.Discard, verbosityLevel))
	case WarningSeverity:
		grpclog.SetLoggerV2(grpclog.NewLoggerV2WithVerbosity(ioutil.Discard, file, ioutil.Discard, verbosityLevel))
	case ErrorSeverity:
		grpclog.SetLoggerV2(grpclog.NewLoggerV2WithVerbosity(ioutil.Discard, ioutil.Discard, file, verbosityLevel))
	default:
		return ErrUnknownSeverity
	}

	logComponent = grpclog.Component(component)
	return nil
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
