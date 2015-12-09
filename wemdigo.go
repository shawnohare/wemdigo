package wemdigo

import (
	"log"
	"os"
)

func logger() func(string, ...interface{}) {
	debug := os.Getenv("DEBUG")
	modes := strings.Split(debug, ",")
	var ok bool
	for _, mode := range modes {
		if mode == "wemdigo" {
			ok = true
			break
		}
	}
	if ok {
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
