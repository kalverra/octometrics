package logging

import (
	"io"
	"net/http"
	"os"
	"time"

	"github.com/rs/zerolog"
	"gopkg.in/natefinch/lumberjack.v2"
)

const logTimeFormat = "2006-01-02T15:04:05.000"

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

	err := os.WriteFile(logFileName, []byte{}, 0644)
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
		writers = append(writers, zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: logTimeFormat})
	}

	logLevel, err := zerolog.ParseLevel(logLevelInput)
	if err != nil {
		return zerolog.Logger{}, err
	}

	zerolog.TimeFieldFormat = logTimeFormat
	multiWriter := zerolog.MultiLevelWriter(writers...)
	return zerolog.New(multiWriter).Level(logLevel).With().Timestamp().Logger(), nil
}

// GitHubClientRoundTripper returns a RoundTripper that logs requests and responses to the GitHub API.
func GitHubClientRoundTripper(logger zerolog.Logger) http.RoundTripper {
	return &loggingTransport{
		transport: http.DefaultTransport,
		logger:    logger,
	}
}

type loggingTransport struct {
	transport http.RoundTripper
	logger    zerolog.Logger
}

// RoundTrip logs the request and response details.
func (lt *loggingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	start := time.Now()

	// Log the request details
	lt.logger.Info().
		Str("method", req.Method).
		Str("url", req.URL.String()).
		Msg("GitHub API request started")

	resp, err := lt.transport.RoundTrip(req)
	duration := time.Since(start)

	if err != nil {
		lt.logger.Error().
			Err(err).
			Str("duration", duration.String()).
			Msg("GitHub API request failed")
		return nil, err
	}

	// Log the response details
	lt.logger.Info().
		Int("status", resp.StatusCode).
		Dur("duration", duration).
		Msg("GitHub API request completed")

	return resp, nil
}
