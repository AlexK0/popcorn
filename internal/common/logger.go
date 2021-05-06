package common

import (
	"errors"
	"io/ioutil"
	"os"
	"sync"

	"google.golang.org/grpc/grpclog"
)

type fileLogWriter struct {
	logFile string
	mu      sync.RWMutex
	file    *os.File

	logComponent grpclog.DepthLoggerV2
}

var (
	logWriter = fileLogWriter{logComponent: grpclog.Component("unknown")}

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

func (writer *fileLogWriter) Write(p []byte) (int, error) {
	writer.mu.RLock()
	n, err := writer.file.Write(p)
	writer.mu.RUnlock()
	return n, err
}

func (writer *fileLogWriter) reopenLog(newLogFile string) error {
	var err error = nil
	new_file := os.Stdout
	if len(newLogFile) != 0 {
		new_file, err = os.OpenFile(newLogFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
		if err != nil {
			return err
		}
	}

	writer.mu.Lock()
	if len(writer.logFile) != 0 {
		err = writer.file.Close()
	}
	writer.logFile = newLogFile
	writer.file = new_file
	writer.mu.Unlock()
	return err
}

// LoggerInit ...
func LoggerInit(component string, logFile string, severity string) error {
	err := logWriter.reopenLog(logFile)
	if err != nil {
		return err
	}

	switch severity {
	case InfoSeverity:
		grpclog.SetLoggerV2(grpclog.NewLoggerV2WithVerbosity(&logWriter, ioutil.Discard, ioutil.Discard, 2))
	case WarningSeverity:
		grpclog.SetLoggerV2(grpclog.NewLoggerV2WithVerbosity(ioutil.Discard, &logWriter, ioutil.Discard, 2))
	case ErrorSeverity:
		grpclog.SetLoggerV2(grpclog.NewLoggerV2WithVerbosity(ioutil.Discard, ioutil.Discard, &logWriter, 2))
	default:
		return ErrUnknownSeverity
	}

	logWriter.logComponent = grpclog.Component(component)
	return nil
}

// RotateLogFile
func RotateLogFile() error {
	return logWriter.reopenLog(logWriter.logFile)
}

// GetLogFileName ...
func GetLogFileName() string {
	return logWriter.logFile
}

// LogInfo ...
func LogInfo(v ...interface{}) {
	logWriter.logComponent.Info(v...)
}

// LogWarning ...
func LogWarning(v ...interface{}) {
	logWriter.logComponent.Warning(v...)
}

// LogError ...
func LogError(v ...interface{}) {
	logWriter.logComponent.Error(v...)
}

// LogFatal ...
func LogFatal(v ...interface{}) {
	logWriter.logComponent.Fatal(v...)
}
