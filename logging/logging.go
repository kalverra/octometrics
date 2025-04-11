package logging

import (
	"io"
	"os"
	"sync"

	"github.com/rs/zerolog"
	"gopkg.in/natefinch/lumberjack.v2"
)

const TimeLayout = "2006-01-02T15:04:05.000"

var once sync.Once

type options struct {
	disableConsoleLog bool
	logLevelInput     string
	logFileName       string
}

type Option func(*options)

// WithFileName sets the log file name.
func WithFileName(logFileName string) Option {
	return func(o *options) {
		o.logFileName = logFileName
	}
}

// WithLevel sets the log level.
func WithLevel(logLevelInput string) Option {
	return func(o *options) {
		o.logLevelInput = logLevelInput
	}
}

// WithDisableConsoleLog disables console logging.
func DisableConsoleLog() Option {
	return func(o *options) {
		o.disableConsoleLog = true
	}
}

func defaultOptions() *options {
	return &options{
		disableConsoleLog: false,
		logLevelInput:     "info",
		logFileName:       "octometrics.log.json",
	}
}

// New initializes a new logger with the specified options.
func New(options ...Option) (zerolog.Logger, error) {
	opts := defaultOptions()
	for _, opt := range options {
		opt(opts)
	}

	var (
		logFileName       = opts.logFileName
		logLevelInput     = opts.logLevelInput
		disableConsoleLog = opts.disableConsoleLog
	)

	err := os.WriteFile(logFileName, []byte{}, 0600)
	if err != nil {
		return zerolog.Logger{}, err
	}

	lumberLogger := &lumberjack.Logger{
		Filename:   logFileName,
		MaxSize:    100, // megabytes
		MaxBackups: 10,
		MaxAge:     30,
	}

	writers := []io.Writer{lumberLogger}
	if !disableConsoleLog {
		writers = append(writers, zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: TimeLayout})
	}

	logLevel, err := zerolog.ParseLevel(logLevelInput)
	if err != nil {
		return zerolog.Logger{}, err
	}

	once.Do(func() {
		zerolog.TimeFieldFormat = TimeLayout
	})
	multiWriter := zerolog.MultiLevelWriter(writers...)
	return zerolog.New(multiWriter).Level(logLevel).With().Timestamp().Logger(), nil
}
