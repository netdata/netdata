// SPDX-License-Identifier: GPL-3.0-or-later

package mysql

import (
	"fmt"
	"os"

	"gopkg.in/ini.v1"
)

func dsnFromFile(filename string) (string, error) {
	f, err := ini.Load(filename)
	if err != nil {
		return "", err
	}

	section, err := f.GetSection("client")
	if err != nil {
		return "", err
	}

	defaultUser := getUser()
	defaultHost := "localhost"
	defaultPort := "3306"

	user := section.Key("user").String()
	password := section.Key("password").String()
	socket := section.Key("socket").String()
	host := section.Key("host").String()
	port := section.Key("port").String()
	database := section.Key("database").String()

	var dsn string

	if user != "" {
		dsn = user
	} else {
		dsn = defaultUser
	}

	if password != "" {
		dsn += ":" + password
	}

	switch {
	case socket != "":
		dsn += fmt.Sprintf("@unix(%s)/", socket)
	case host != "" && port != "":
		dsn += fmt.Sprintf("@tcp(%s:%s)/", host, port)
	case host != "":
		dsn += fmt.Sprintf("@tcp(%s:%s)/", host, defaultPort)
	case port != "":
		dsn += fmt.Sprintf("@tcp(%s:%s)/", defaultHost, port)
	default:
		dsn += "@/"
	}

	if database != "" {
		dsn += database
	}
	return dsn, nil
}

func getUser() (user string) {
	if user = os.Getenv("LOGNAME"); user != "" {
		return user
	}
	if user = os.Getenv("USER"); user != "" {
		return user
	}
	if user = os.Getenv("LNAME"); user != "" {
		return user
	}
	if user = os.Getenv("USERNAME"); user != "" {
		return user
	}
	return ""
}
