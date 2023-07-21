package log

import (
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	"github.com/hashicorp/yamux"
)

type YamuxLogr struct {
	Logger logr.Logger
}

var _ yamux.Logger = &YamuxLogr{}

// Print implements yamux.Logger.
func (yl *YamuxLogr) Print(v ...interface{}) {
	yl.Printf(fmt.Sprint(v...))
}

// Printf implements yamux.Logger.
func (yl *YamuxLogr) Printf(format string, v ...interface{}) {
	errorPrefix := "[ERR] "
	warnPrefix := "[WARN] "

	switch {
	case strings.HasPrefix(format, errorPrefix):
		format = strings.TrimPrefix(format, errorPrefix)

		var err error

		for _, entry := range v {
			localErr, isError := entry.(error)
			if isError {
				err = localErr
				break
			}
		}

		yl.Logger.Error(err, fmt.Sprintf(format, v...))
	case strings.HasPrefix(format, warnPrefix):
		format = strings.TrimPrefix(format, warnPrefix)

		yl.Logger.Info(fmt.Sprintf(format, v...))
	default:
		yl.Logger.V(1).Info(fmt.Sprintf(format, v...))
	}
}

// Println implements yamux.Logger.
func (yl *YamuxLogr) Println(v ...interface{}) {
	yl.Printf(fmt.Sprintln(v...))
}
