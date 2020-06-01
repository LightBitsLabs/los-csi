// +build have_net,have_nvme

package cli_test

import (
	"strings"
	"testing"

	"github.com/lightbitslabs/lb-csi/pkg/nvme-of/client"
	"github.com/lightbitslabs/lb-csi/pkg/nvme-of/client/cli"
	"github.com/sirupsen/logrus"
)

var base = map[string]string{
	"trtype":  "tcp",
	"subnqn":  "nqn.2014-08.com.example",
	"traddr":  "example.com",
	"trsvcid": "4420",
	"hostnqn": "4ee76060-7e90-4b0c-afcc-7a8a4353776c",
}

var testCases = []map[string]string{
	{"trtype": ""},
	{"trtype": "fc"},
	{"subnqn": ""},
	{"traddr": ""},
	{"traddr": "no.such.host.no.such.domain"},
	{"traddr": "1.2.3"},
	{"traddr": "1.2.3.4:8080"},
	{"traddr": "tcp://8.8.8.8"},
	{"trsvcid": ""},
	{"trsvcid": "abc"},
	{"trsvcid": "0x1144"},
	{"trtype": "", "subnqn": "", "traddr": "", "trsvcid": ""},
	{"trtype": "fc", "subnqn": "", "traddr": "1.2.3", "trsvcid": "0xDEAD"},
}

func TestConnectParams(t *testing.T) {
	for i, tt := range testCases {
		args := make(map[string]string)
		bad := []string{}
		for k, v := range base {
			args[k] = v
			if val, ok := tt[k]; ok {
				args[k] = val
				bad = append(bad, k)
			}
		}
		t.Run("bad:"+strings.Join(bad, ","), func(t *testing.T) {
			if testing.Verbose() {
				for _, k := range bad {
					t.Logf("passing bad '%s' arg: '%s'", k, args[k])
				}
			}
			rc, err := cli.New(logrus.New().WithFields(logrus.Fields{"TEST#": i}))
			if err != nil {
				t.Fatalf("FAIL: failed to create nvme-cli wrapper: %s", err)
			}
			var c client.Client = rc
			err = c.Connect(args["trtype"], args["subnqn"], args["traddr"],
				args["trsvcid"], args["hostnqn"])
			switch err := err.(type) {
			case *cli.BadArgError:
				if testing.Verbose() {
					t.Logf("Connect() failed with BadArgError: '%s'", err.Error())
				}
			default:
				t.Errorf("Connect() failed with unexpected error, '%T': '%s'", err, err.Error())
			case nil:
				t.Error("OMG: EVERYTHING WORKED! THAT CAN'T BE RIGHT!")
			}
		})
	}
}
