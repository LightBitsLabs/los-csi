package driver

import (
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/sirupsen/logrus"
)

const (
	discoveryClientConfigFolder   = "/etc/discovery-client/discovery.d"
	discoveryClientReservedPrefix = "tmp.dc."
)

type entry struct {
	transport string
	traddr    string
	trsvcid   int
	hostnqn   string
	nqn       string
}

func createEntries(addresses []string, hostnqn string, nqn string, transport string) ([]*entry, error) {
	var entries []*entry
	for _, address := range addresses {
		splitted := strings.Split(address, ":")
		if len(splitted) != 2 {
			return nil, fmt.Errorf("address should be of format: <address>:<port>")
		}
		ipAddress := net.ParseIP(splitted[0])
		if ipAddress.To4() == nil {
			return nil, fmt.Errorf("%v is not an IPv4 address", splitted[0])
		}
		port, err := strconv.ParseUint(splitted[1], 10, 16)
		if err != nil {
			return nil, err
		}
		e := &entry{
			transport: transport,
			traddr:    ipAddress.String(),
			trsvcid:   int(port),
			hostnqn:   hostnqn,
			nqn:       nqn,
		}
		entries = append(entries, e)
	}
	return entries, nil
}

func entriesToString(entries []*entry) string {
	var b strings.Builder
	for _, entry := range entries {
		b.WriteString(fmt.Sprintf("-t %s -a %s -s %d -q %s -n %s\n", entry.transport, entry.traddr, entry.trsvcid, entry.hostnqn, entry.nqn))
	}
	return b.String()
}

func writeEntriesToFile(filename string, entries []*entry) error {
	filepath := path.Join(discoveryClientConfigFolder, filename)
	content := []byte(entriesToString(entries))
	tmpfile, err := ioutil.TempFile(discoveryClientConfigFolder, discoveryClientReservedPrefix)
	if err != nil {
		logrus.WithError(err).Errorf("failed to create temp file")
		return err
	}
	defer os.Remove(tmpfile.Name()) // clean up
	if _, err := tmpfile.Write(content); err != nil {
		logrus.WithError(err).Errorf("failed to write to temp file")
		return err
	}
	if err := tmpfile.Close(); err != nil {
		logrus.WithError(err).Errorf("failed to close temp file")
		return err
	}
	err = os.Rename(tmpfile.Name(), filepath)
	if err != nil {
		logrus.WithError(err).Errorf("failed to mv %s to %s", tmpfile.Name(), filepath)
		return err
	}
	return nil
}

func deleteDiscoveryEntriesFile(filename string) error {
	filepath := path.Join(discoveryClientConfigFolder, filename)
	logrus.Infof("deleting file %q", filepath)
	if err := os.RemoveAll(filepath); err != nil {
		return err
	}
	return nil
}
