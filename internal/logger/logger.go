// Package logger provides a single logger for use by various GopherCI packages.
//
// It's designed to hide a single concrete logger from the various packages, it's
// not designed to provide many logger alternatives to be swapped.
package logger

import (
	"io"
	"os"
	"time"

	"github.com/evalphobia/logrus_sentry"
	"github.com/sirupsen/logrus"
)

// Logger is a service to write structured, levelled logs with context.
type Logger interface {
	// Debug level for developer concerned debugging, not visible in production.
	Debug(args ...interface{})
	Debugf(format string, args ...interface{})

	// Info logs general events.
	Info(args ...interface{})
	Infof(format string, args ...interface{})

	// Error logs, errors. An error should only be logged once.
	Error(args ...interface{})
	Errorf(format string, args ...interface{})

	// Fatal logs an error and then immediately terminates execution.
	Fatal(args ...interface{})
	Fatalf(format string, args ...interface{})

	// With adds context to a logger.
	With(name string, value interface{}) Logger
}

// Log implements the Logger interface by wrapping logrus.
type log struct {
	logrus *logrus.Entry
}

// New constructs a new Logger.
func New(out io.Writer, build, env, sentryDSN string) Logger {
	logger := logrus.New()
	logger.Out = out
	switch env {
	case "production":
		logger.Formatter = &logrus.JSONFormatter{}
		logger.Level = logrus.InfoLevel
	default:
		logger.Formatter = &logrus.TextFormatter{}
		logger.Level = logrus.DebugLevel
	}

	// server_name and logger have special meanings to logrus_sentry, to add that as a tag
	ctxLogger := logger.WithField("logger", "gci")
	if hostname, err := os.Hostname(); err == nil {
		ctxLogger = ctxLogger.WithField("server_name", hostname)
	}

	if sentryDSN != "" {
		hook, err := logrus_sentry.NewSentryHook(sentryDSN, []logrus.Level{
			logrus.PanicLevel,
			logrus.FatalLevel,
			logrus.ErrorLevel,
		})
		hook.SetEnvironment(env)
		hook.SetRelease(build)
		hook.StacktraceConfiguration.Enable = true
		hook.StacktraceConfiguration.Level = logrus.ErrorLevel // defaults to panic
		hook.Timeout = 1 * time.Second                         // 100ms default is often too low
		if err != nil {
			logger.WithError(err).Fatal("could not setup sentry logrus")
		}
		logger.Hooks.Add(hook)
		ctxLogger.WithField("area", "logger").Info("enabled sentry")
	}

	return &log{
		logrus: ctxLogger,
	}
}

// Testing returns a logger for use in tests.
func Testing() Logger {
	return New(os.Stdout, "", "testing", "")
}

// Debug implements the Logger interface.
func (l *log) Debug(args ...interface{}) {
	l.logrus.Debug(args...)
}

// Debugf implements the Logger interface.
func (l *log) Debugf(format string, args ...interface{}) {
	l.logrus.Debugf(format, args...)
}

// Info implements the Logger interface.
func (l *log) Info(args ...interface{}) {
	l.logrus.Info(args...)
}

// Infof implements the Logger interface.
func (l *log) Infof(format string, args ...interface{}) {
	l.logrus.Infof(format, args...)
}

// Error implements the Logger interface.
func (l *log) Error(args ...interface{}) {
	l.logrus.Error(args...)
}

// Errorf implements the Logger interface.
func (l *log) Errorf(format string, args ...interface{}) {
	l.logrus.Errorf(format, args...)
}

// Fatal implements the Logger interface.
func (l *log) Fatal(args ...interface{}) {
	l.logrus.Fatal(args...)
}

// Fatalf implements the Logger interface.
func (l *log) Fatalf(format string, args ...interface{}) {
	l.logrus.Fatalf(format, args...)
}

// With implements the Logger interface.
func (l *log) With(key string, value interface{}) Logger {
	return &log{
		logrus: l.logrus.WithField(key, value),
	}
}
