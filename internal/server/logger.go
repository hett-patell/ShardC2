package server

import (
	"encoding/json"
	"io"
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
	json.NewEncoder(l.writer).Encode(entry)
}

func (l *Logger) Info(category, message string, fields map[string]interface{}) {
	l.log("info", category, message, fields)
}

func (l *Logger) shouldLog(level string) bool {
	// Simple level check
	return true // implement properly
}
