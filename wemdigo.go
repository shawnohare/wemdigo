package wemdigo

import (
	"log"
	"os"
	"strings"
)

func logger() func(string, ...interface{}) {
	if strings.Contains(os.Getenv("DEBUG"), "wemdigo") {
		f := func(format string, v ...interface{}) {
			wemdigoStr := "\033[0;32m" + "wemdigo" + "\033[0m "
			log.Printf(wemdigoStr+format, v...)
		}
		return f
	}
	f := func(format string, v ...interface{}) {
		return
	}
	return f
}

var dlog = logger()
