// Copyright (C) 2021 Lightbits Labs Ltd.
// SPDX-License-Identifier: Apache-2.0

package backend

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v2"
)

// ConfigBase is the mandatory base part of backend-specific configs for all
// backends. it is used primarily to detect which backend is to be used.
//
// the ConfigBase struct should be embedded in backend-specific config structs
// as follows:
//   type MyBackendConfig struct {
//           ConfigBase `yaml:",inline"`  // note `inline`!
//           MyPath string `yaml:"my-path"`
//           MyNum  uint32 `yaml:"num"`
//   }
type ConfigBase struct {
	Backend string `yaml:"backend"`
}

// FmtYAMLError works around `yaml.v2` package's unfortunate choice of multi-
// line and indented default error formatting, which is unsuitable for logging.
func FmtYAMLError(err error) string {
	if err == nil {
		return "<nil>"
	}
	if ye, ok := err.(*yaml.TypeError); ok {
		return strings.Join(ye.Errors, ", ")
	}
	return err.Error()
}

func DetectType(rawCfg []byte) (string, error) {
	var base ConfigBase
	err := yaml.Unmarshal(rawCfg, &base)
	if err != nil {
		return "", fmt.Errorf("failed to parse: %s", FmtYAMLError(err))
	}
	be := strings.TrimSpace(base.Backend)
	if be == "" {
		return "", fmt.Errorf("backend type missing")
	}
	return be, nil
}
