package loglevel

import (
	"log"
	"sync/atomic"
)

type Level int32

const (
	LevelSilent Level = iota
	LevelInfo
	LevelVerbose
	LevelDebug
)

var current atomic.Int32

func init() {
	current.Store(int32(LevelInfo))
}

func Set(level Level) {
	current.Store(int32(level))
}

func Get() Level {
	return Level(current.Load())
}

func Debugf(format string, args ...any) {
	if Get() >= LevelDebug {
		log.Printf("debug: "+format, args...)
	}
}

func Verbosef(format string, args ...any) {
	if Get() >= LevelVerbose {
		log.Printf("verbose: "+format, args...)
	}
}

func Infof(format string, args ...any) {
	if Get() >= LevelInfo {
		log.Printf("info: "+format, args...)
	}
}
