package log

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"

	"cloud.google.com/go/logging"
	"github.com/jonstaryuk/gcloudzap"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"google.golang.org/api/option"
)

// Configuration is the primary config for logging
type Configuration struct {
	Console            bool
	ConsoleLevel       zapcore.Level
	GoogleCloudProject string
	GoogleCloudCreds   string
	FilePath           string
	FileLevel          zapcore.Level
}

// sugared is the primary log interface for Wasabee-Server
var sugared *zap.SugaredLogger

// SetupLogging is called very early by the startup routine to configure logging
func SetupLogging(c *Configuration) {
	var cores []zapcore.Core

	if c.Console {
		atom := zap.NewAtomicLevel()
		atom.SetLevel(c.ConsoleLevel)
		encoderCfg := zap.NewDevelopmentEncoderConfig()
		encoderCfg.EncodeLevel = zapcore.CapitalColorLevelEncoder
		consoleCore := zapcore.NewCore(
			zapcore.NewConsoleEncoder(encoderCfg),
			zapcore.Lock(os.Stdout),
			atom,
		)
		cores = append(cores, consoleCore)
	}

	if c.FilePath != "" {
		fileCore, err := addFileLog(c.FilePath, c.FileLevel)
		if err != nil {
			fmt.Printf("Unable to open log file, %s: %v\n", c.FilePath, err)
		} else {
			cores = append(cores, fileCore)
		}
	}

	if c.GoogleCloudProject != "" && c.GoogleCloudCreds != "" {
		gcCore, err := addGoogleCloud(c.GoogleCloudProject, c.GoogleCloudCreds)
		if err != nil {
			fmt.Printf("unable to start cloud logging to project %s with creds %s: %v\n", c.GoogleCloudProject, c.GoogleCloudCreds, err)
		} else {
			cores = append(cores, gcCore)
		}
	}

	tee := zapcore.NewTee(cores...)
	sugarfree := zap.New(tee, zap.AddCaller(), zap.AddCallerSkip(1), zap.AddStacktrace(zap.FatalLevel)) // zap.FatalLevel
	undo, err := zap.RedirectStdLogAt(sugarfree, zap.DebugLevel)
	if err != nil {
		undo()
	}

	sugared = sugarfree.Sugar()
}

func addGoogleCloud(project string, jsonPath string) (zapcore.Core, error) {
	ctx := context.Background()
	opt := option.WithCredentialsFile(jsonPath)

	atom := zap.NewAtomicLevel()
	atom.SetLevel(zap.InfoLevel)
	encoderCfg := zap.NewProductionEncoderConfig()
	inCore := zapcore.NewCore(
		zapcore.NewJSONEncoder(encoderCfg),
		zapcore.AddSync(ioutil.Discard),
		atom,
	)

	client, err := logging.NewClient(ctx, project, opt)
	if err != nil {
		return nil, err
	}

	hn, err := os.Hostname()
	if err != nil {
		hn = "wasabee-server"
	}
	gcore := gcloudzap.Tee(inCore, client, hn)
	return gcore, nil
}

// addFileLog duplicates the console log to a file
func addFileLog(logfile string, level zapcore.Level) (zapcore.Core, error) {
	// #nosec
	lf, err := os.OpenFile(logfile, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return nil, err
	}

	atom := zap.NewAtomicLevel()
	atom.SetLevel(level)

	encoderCfg := zap.NewDevelopmentEncoderConfig()
	fileCore := zapcore.NewCore(
		zapcore.NewConsoleEncoder(encoderCfg),
		zapcore.Lock(lf),
		atom,
	)

	return fileCore, nil
}

// Debug logs at the lowest level
func Debug(args ...interface{}) {
	sugared.Debug(args...)
}

// Debugw logs structured logs at the lowest level
func Debugw(msg string, args ...interface{}) {
	sugared.Debugw(msg, args...)
}

// Error logs at the level which requires attention
func Error(args ...interface{}) {
	sugared.Error(args...)
}

// Errorw logs structured logs at the level which requires attention
func Errorw(msg string, args ...interface{}) {
	sugared.Errorw(msg, args...)
}

// Fatal logs a message and stops the process
func Fatal(args ...interface{}) {
	sugared.Fatal(args...)
}

// Fatalw logs a structured log and stops the process
func Fatalw(msg string, args ...interface{}) {
	sugared.Fatalw(msg, args...)
}

// Info logs messages which are helpful for tracking problems
func Info(args ...interface{}) {
	sugared.Info(args...)
}

// Infow logs structured logs which are helpful for tracking problems
func Infow(msg string, args ...interface{}) {
	sugared.Infow(msg, args...)
}

// Panic logs critical messages and stops the process
func Panic(args ...interface{}) {
	sugared.Panic(args...)
}

// Panicw logs structured logs and stops the process
func Panicw(msg string, args ...interface{}) {
	sugared.Panicw(msg, args...)
}

// Warn logs unusual situations
func Warn(args ...interface{}) {
	sugared.Warn(args...)
}

// Warnw logs strucutured logs for unusual situations
func Warnw(msg string, args ...interface{}) {
	sugared.Warnw(msg, args...)
}
