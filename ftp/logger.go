package ftp

import "fmt"

func NewConsoleLogger() Logger {
	return consoleLogger{}
}

type consoleLogger struct{}

func (consoleLogger) SentFTP(msg []byte, err error) {
	write("--->", msg, err)
}

func (consoleLogger) ReceivedFTP(response []byte, err error) {
	write("<---", response, err)
}

func write(arrow string, msg []byte, err error) {
	fmt.Print(arrow, " ")
	if err != nil {
		fmt.Print("ERROR:", err.Error())
	} else {
		fmt.Print(string(msg))
	}
}
