// Copyright (C) 2016--2020 Lightbits Labs Ltd.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	flag "github.com/spf13/pflag"

	"github.com/lightbitslabs/los-csi/pkg/driver"
)

const usageTemplate = `USAGE: {{.BinaryName}} [flags]

{{.BinaryName}} is an implementation of the Container Storage Interface (CSI)
plugin for Container Orchestration (CO) systems. For details, see:
    https://github.com/container-storage-interface/spec

Officially supported CSI plugin configuration is obtained primarily from
environment variables. Command-line flags can be used to override the
environment configuration and to tweak various debugging options.

Supported environment variables:
  CSI_ENDPOINT      - URL of gRPC endpoint used to communicate with this
        plugin. Currently only Unix Domain Socket (UDS) endpoints are supported.
        (default: {{.Endpoint}})
  LB_CSI_NODE_ID    - Cluster Node ID mustn't be empty and should be unique
        among all the Node plugin instances in a cluster. CO node name is
        usually a good candidate for a Node ID.
  LB_CSI_DEFAULT_FS - one of: {ext4}. Unless otherwise specified, volumes with
        no FS on them will be formatted to this FS before being mounted.
        (default: {{.DefaultFS}})
  LB_CSI_LOG_LEVEL  - one of: {debug, info, warning, error}. Minimal entry
        severity level to log. (default: {{.LogLevel}})
  LB_CSI_LOG_ROLE   - one of: {node, controller}. Aids monitoring by allowing
        to distinguish between separate instances of the plugin serving
        different CSI core services on the same CO node. (default: {{.LogRole}})
  LB_CSI_LOG_TIME   - one of: {true, false}. Attach explicit timestamps to log
        entries. May be redundant in some monitoring environments that
        automatically timestamp log entries. (default: {{.LogTimestamps}})
  LB_CSI_LOG_FMT    - one of: {text, json}. (default: {{.LogFormat}})
  LB_CSI_JWT_PATH   - path to the file storing a JWT to be used for authN/authZ
        with LightOS. this JWT will only be used as a fallback if a per-call JWT
        is not specified through the CSI API, e.g. in global, plugin-wide
        authentication configuration. (default: {{.JWTPath}})
  LB_CSI_BE_CONFIG_PATH     - path to the NVMe-oF host backend configuration
        file, in YAML format. the value of the top-level 'backend' key in this
        file determines which backend to use, the rest of the keys/values are
        a backend-specific configuration. if the specified file does not exist -
        the '{{.DefaultBackend}}' backend with default configuration will be used.
        runtime backend configuration changes are not supported, to reload the
        config - restart the plugin. (default: {{.BackendCfgPath}})

Command line flags:
`

const (
	defaultCfgDirPath         = "/etc/lb-csi"
	defaultJWTFileName        = "jwt"
	defaultBackendCfgFileName = "backend.yaml"
)

var defaults = driver.Config{
	DefaultBackend: "dsc",
	BackendCfgPath: filepath.Join(defaultCfgDirPath, defaultBackendCfgFileName),
	JWTPath:        filepath.Join(defaultCfgDirPath, defaultJWTFileName),

	NodeID:   "",
	Endpoint: "unix:///tmp/csi.sock",

	DefaultFS: "ext4",

	LogLevel:      "info",
	LogRole:       "node",
	LogTimestamps: false,
	LogFormat:     "json",

	// hidden, dev-only options:
	BinaryName:    "lb-csi-plugin",
	Transport:     "tcp",
	SquelchPanics: false,
	PrettyJson:    false,
}

var (
	nodeID = flag.StringP("node-id", "n", "",
		"Cluster Node ID, see $LB_CSI_NODE_ID.")
	endpoint = flag.StringP("endpoint", "e", "",
		"CSI endpoint, see $CSI_ENDPOINT.")
	defaultFS = flag.StringP("default-fs", "F", "",
		"Default FS for unformatted volumes, see $LB_CSI_DEFAULT_FS.")
	logLevel = flag.StringP("log-level", "l", "",
		"Log severity, see $LB_CSI_LOG_LEVEL.")
	logRole = flag.StringP("log-role", "r", "",
		"Plugin instance role to log, see $LB_CSI_LOG_ROLE.")
	logTimestamps = flag.BoolP("log-time", "T", false,
		"Add timestamps to log entries, see $LB_CSI_LOG_TIME.")
	logFormat = flag.StringP("log-fmt", "f", "",
		"Log entry format, see $LB_CSI_LOG_FMT.")
	jwtPath = flag.StringP("jwt-path", "j", "",
		"Path to global LightOS API auth JWT, see $LB_CSI_JWT_PATH.")
	backendCfgPath = flag.StringP("be-cfg-path", "b", "",
		"Backend config path, see $LB_CSI_BE_CONFIG_PATH.")
	version = flag.Bool("version", false, "Print the version and exit.")
	help    = flag.BoolP("help", "h", false, "Print help and exit.")

	// hidden, dev-only options:
	transport = flag.StringP("transport", "t", "tcp",
		"Transport to use for connection to storage. One of: {tcp, rdma}.")
	squelchPanics = flag.BoolP("squelch-panics", "P", defaults.SquelchPanics,
		"Recover panics and return them to the remote client as gRPC "+
			"errors. NOT safe for use in production environments!")
	prettyJson = flag.BoolP("pretty-json", "J", defaults.PrettyJson,
		"Pretty-print JSON log output, with indentations and all. "+
			"Useful mainly for dev/test as this bloats the logs "+
			"even more than they already are. Has no effect on "+
			"test log formatter.")
)

func usageAndDie() {
	t := template.Must(template.New("usage").Parse(usageTemplate))
	usageBuf := new(bytes.Buffer)
	err := t.Execute(usageBuf, defaults)
	if err != nil {
		fmt.Fprintf(os.Stderr, "\nOops, fumbled usage. please report this!\n\n")
	} else {
		fmt.Fprint(os.Stderr, usageBuf.String())
	}
	flagsHelp := flag.CommandLine.FlagUsagesWrapped(80)
	fmt.Fprint(os.Stderr, flagsHelp)
	os.Exit(2)
}

func errorAndDie(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "ERROR: "+format+"\n", args...)
	fmt.Fprintf(os.Stderr, "\nTry '%s --help' for more information.\n",
		defaults.BinaryName)
	os.Exit(2)
}

// populate config from: flags, env vars, defaults in that order:
func pickStr(flagVal string, envVar string, def string) string {
	res := flagVal
	if res == "" {
		res = os.Getenv(envVar)
		if res == "" {
			res = def
		}
	}
	return res
}

func main() {
	flag.CommandLine.Init(os.Args[0], flag.ContinueOnError)
	flag.CommandLine.MarkHidden("transport")
	flag.CommandLine.MarkHidden("squelch-panics")
	flag.CommandLine.MarkHidden("pretty-json")
	flag.SetInterspersed(false)
	err := flag.CommandLine.Parse(os.Args[1:])
	if err != nil {
		errorAndDie(err.Error())
		os.Exit(2)
	}
	if *help {
		usageAndDie()
	}
	if *version {
		fmt.Printf("%s %s\n", defaults.BinaryName, driver.GetFullVersionStr())
		os.Exit(0)
	}

	if !*logTimestamps {
		val := os.Getenv("LB_CSI_LOG_TIME")
		switch strings.ToLower(strings.TrimSpace(val)) {
		case "true":
			*logTimestamps = true
		case "false":
			*logTimestamps = false
		case "":
			*logTimestamps = defaults.LogTimestamps
		default:
			errorAndDie("invalid LB_CSI_LOG_TIME value: '%s'", val)
		}
	}

	cfg := driver.Config{
		DefaultBackend: defaults.DefaultBackend, // not user configurable.
		BackendCfgPath: pickStr(*backendCfgPath, "LB_CSI_BE_CONFIG_PATH",
			defaults.BackendCfgPath),
		JWTPath:       pickStr(*jwtPath, "LB_CSI_JWT_PATH", defaults.JWTPath),
		NodeID:        pickStr(*nodeID, "LB_CSI_NODE_ID", defaults.NodeID),
		Endpoint:      pickStr(*endpoint, "CSI_ENDPOINT", defaults.Endpoint),
		DefaultFS:     pickStr(*defaultFS, "LB_CSI_DEFAULT_FS", defaults.DefaultFS),
		LogLevel:      pickStr(*logLevel, "LB_CSI_LOG_LEVEL", defaults.LogLevel),
		LogRole:       pickStr(*logRole, "LB_CSI_LOG_ROLE", defaults.LogRole),
		LogFormat:     pickStr(*logFormat, "LB_CSI_LOG_FMT", defaults.LogFormat),
		LogTimestamps: *logTimestamps,
		Transport:     *transport,
		SquelchPanics: *squelchPanics,
		PrettyJson:    *prettyJson,
	}

	d, err := driver.New(cfg)
	if err != nil {
		errorAndDie(err.Error())
	}

	if err := d.Run(); err != nil {
		errorAndDie(err.Error())
	}
}
