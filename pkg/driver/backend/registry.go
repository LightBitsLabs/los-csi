package backend

import (
	"fmt"
	"log/slog"
	"regexp"
)

type MakerFn func(log *slog.Logger, hostNQN string, rawCfg []byte) (Backend, error)

type regEntry struct {
	beType string
	maker  MakerFn
}

var (
	beRegistry  = make(map[string]regEntry)
	beTypeRegex *regexp.Regexp
)

const (
	beTypeTemplate = `^[a-z]([a-z0-9-]{0,30}[a-z0-9])?$`
)

func init() {
	beTypeRegex = regexp.MustCompile(beTypeTemplate)
}

// RegisterBackend registers backend constructors with the backend factory.
// backends are expected to call it from their init() functions. `beType` must
// comply with `beTypeRegex`.
//
// since it is invoked at LB CSI plugin initialisation time in production,
// it will panic if a caller will attempt to register a duplicate `beType` or
// an invalid beType.
func RegisterBackend(beType string, maker MakerFn) {
	if !beTypeRegex.MatchString(beType) {
		panic(fmt.Sprintf("attempt to register invalid backend type '%s', "+
			"name must comply with template: '%s'", beType, beTypeTemplate))
	}
	if _, ok := beRegistry[beType]; ok {
		panic(fmt.Sprintf("attempt to register backend type '%s' more than once", beType))
	}
	beRegistry[beType] = regEntry{beType, maker}
}

func Make(beType string, log *slog.Logger, hostNQN string, rawCfg []byte) (Backend, error) {
	if re, ok := beRegistry[beType]; ok {
		return NewWrapper(beType, re.maker, log, hostNQN, rawCfg)
	}
	return nil, fmt.Errorf("unsupported backend type: '%s'", beType)
}

func ListBackends() []string {
	res := []string{}
	for k := range beRegistry {
		res = append(res, k)
	}
	return res
}
