package server

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

type Logger struct {
	writer io.Writer
	level  string
}

type LogEntry struct {
	Timestamp time.Time              `json:"timestamp"`
	Level     string                 `json:"level"`
	Category  string                 `json:"category"`
	Message   string                 `json:"message"`
	Fields    map[string]interface{} `json:"fields,omitempty"`
}

func NewLogger(w io.Writer, level string) *Logger {
	return &Logger{writer: w, level: level}
}

func (l *Logger) log(level, category, message string, fields map[string]interface{}) {
	if !l.shouldLog(level) {
		return
	}
	entry := LogEntry{
		Timestamp: time.Now(),
		Level:     level,
		Category:  category,
		Message:   message,
		Fields:    fields,
	}
	if err := json.NewEncoder(l.writer).Encode(entry); err != nil {
		fmt.Fprintf(os.Stderr, "Logger error: %v\n", err)
	}
}

func (l *Logger) Info(category, message string, fields map[string]interface{}) {
	l.log("info", category, message, fields)
}

func (l *Logger) Debug(category, message string, fields map[string]interface{}) {
	l.log("debug", category, message, fields)
}

func (l *Logger) Warn(category, message string, fields map[string]interface{}) {
	l.log("warn", category, message, fields)
}

func (l *Logger) Error(category, message string, fields map[string]interface{}) {
	l.log("error", category, message, fields)
}

func (l *Logger) shouldLog(level string) bool {
	levelMap := map[string]int{
		"debug": 0,
		"info":  1,
		"warn":  2,
		"error": 3,
	}
	levelInt, ok := levelMap[strings.ToLower(level)]
	if !ok {
		return false // unknown level, don't log
	}
	configuredInt, ok := levelMap[strings.ToLower(l.level)]
	if !ok {
		return true // unknown configured level, log all
	}
	return levelInt >= configuredInt
}
