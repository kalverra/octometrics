package logging

import (
	"io"
	"os"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"gopkg.in/natefinch/lumberjack.v2"
)

const logTimeFormat = "2006-01-02T15:04:05.000"

func Setup(logFileName, logLevelInput string, disableConsoleLog bool) error {
	err := os.WriteFile(logFileName, []byte{}, 0644)
	if err != nil {
		return err
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
		return err
	}

	zerolog.TimeFieldFormat = logTimeFormat
	multiWriter := zerolog.MultiLevelWriter(writers...)
	log.Logger = zerolog.New(multiWriter).Level(logLevel).With().Timestamp().Logger()
	return nil
}
