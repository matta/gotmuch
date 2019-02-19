package homedir

import (
	"os"
	"os/user"
)

func Get() string {
	h := os.Getenv("HOME")
	if h != "" {
		return h
	}

	usr, err := user.Current()
	if err != nil {
		panic(err)
	}
	return usr.HomeDir
}
