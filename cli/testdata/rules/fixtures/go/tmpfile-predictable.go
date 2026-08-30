package fixture

import (
	"io/ioutil"
	"os"
)

func stash(data []byte) error {
	// ruleid: go-tmpfile-predictable
	return ioutil.WriteFile("/tmp/session.cache", data, 0o600)
}

func touch() (*os.File, error) {
	// ruleid: go-tmpfile-predictable
	return os.Create("/tmp/build.lock")
}

func write(data []byte) error {
	// ruleid: go-tmpfile-predictable
	return os.WriteFile("/tmp/report.json", data, 0o600)
}

func proper(data []byte) error {
	f, err := os.CreateTemp("", "session")
	if err != nil {
		return err
	}
	defer f.Close()
	// ok: go-tmpfile-predictable
	_, err = f.Write(data)
	return err
}

func inHome(data []byte) error {
	// ok: go-tmpfile-predictable
	return os.WriteFile("/var/lib/app/report.json", data, 0o600)
}
