// Copyright (C) 2016--2020 Lightbits Labs Ltd.
// SPDX-License-Identifier: Apache-2.0

package driver

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/lightbitslabs/los-csi/pkg/util/endpoint"
)

type testCase struct {
	id string
	pr string
	sc string
}

//nolint:lll
var goodIDs = []testCase{
	{id: "mgmt:1.2.3.4:80|nguid:00000000-0000-0000-0000-000000000001"}, // keep first
	{id: "mgmt:1.2.3.4:80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66"},
	{id: "mgmt:1.2.3.4:80|nguid:6BB32FB5-99AA-4A4C-A4E7-30B7787BBD66"},
	{id: "mgmt:lb01.net:80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66"},
	{id: "mgmt:1.0.0.1:80,1.0.0.2:80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66"},
	{id: "mgmt:lb01.net:80,lb02.net:80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66"},
	{id: "mgmt:1.0.0.1:80,lb02.net:80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66"},
	{id: "mgmt:1.0.0.1:80,1.0.0.2:80,1.0.0.3:80,1.0.0.4:80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66"},
	{id: "mgmt:lb01.net:80,lb02.net:80,lb03.net:80,lb04.net:80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66"},

	{id: "mgmt:1.2.3.4:80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66|proj:3", pr: "3"},
	{id: "mgmt:1.2.3.4:80|nguid:6BB32FB5-99AA-4A4C-A4E7-30B7787BBD66|proj:proj3", pr: "proj3"},
	{id: "mgmt:lb01.net:80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66|proj:v3.4", pr: "v3.4"},
	{id: "mgmt:1.0.0.1:80,1.0.0.2:80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66|proj:r-n-d", pr: "r-n-d"},
	{id: "mgmt:lb01.net:80,lb02.net:80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66|proj:r.n.d-12-06-78", pr: "r.n.d-12-06-78"},
	{id: "mgmt:1.0.0.1:80,lb02.net:80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66|proj:abababababababbabababababba", pr: "abababababababbabababababba"},
	{id: "mgmt:1.0.0.1:80,1.0.0.2:80,1.0.0.3:80,1.0.0.4:80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66|proj:aaa", pr: "aaa"},
	{id: "mgmt:lb01.net:80,lb02.net:80,lb03.net:80,lb04.net:80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66|proj:bbb", pr: "bbb"},

	{id: "mgmt:10.19.151.24:443,10.19.151.6:443|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66|proj:grpcs|scheme:grpcs", pr: "grpcs", sc: "grpcs"},
	{id: "mgmt:10.19.151.24:443,10.19.151.6:443|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66|proj:a|scheme:grpcs", pr: "a", sc: "grpcs"},
	{id: "mgmt:10.19.151.24:443,10.19.151.6:443|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66|scheme:grpcs", sc: "grpcs"},
	{id: "mgmt:10.19.151.24:443,10.19.151.6:443|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66|scheme:grpc", sc: "grpc"},
}

//nolint:lll
var badIDs = []string{
	"",
	"\n",
	"\\0",
	"mgmt:1.2.3.4:80||nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66",
	"mgmt:1.2.3.4:80:900|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66",
	"mgmt:1.2.3.4 80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66",
	"mgmt:1.2.3.4-80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66",
	"nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66|mgmt:1.2.3.4:80",
	"mgmt:1.2.3.4:80| |nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66",
	"nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66",
	"mgmt:1.2.3.4:80|mgmt:5.6.7.8:80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66",
	"mgmt:1.2.3.4:80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66|mgmt:5.6.7.8:80",
	"mgmt:1.2.3.4:80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66",
	"mgmt:1.2.3.4:80",
	"mgmt:1.2.3.4:80|node:|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66",
	"mgmt:1.2.3.4:x|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66",
	"mgmt:1.2.3.4|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66",
	"1.2.3.4:80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66",
	"mgmt:1.2.3.4:80|node|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66",
	"mgmt:1.2.3.4:80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66|volid:17",
	"mgmt:1.2.3.4:80|node:1|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66",
	"mgmt:1.2.3.4:80|node:1|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66|volid:17",
	"mgmt:1.2.3.4:80|nguid:",
	"mgmt:1.2.3.4:80|nguid",
	"mgmt:1.2.3.4:80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66|zorro:x",
	"mgmt:1.2.3.4:80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66|zorro:17",
	"mgmt:sne|aky:80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66",
	"MGMT:1.2.3.4:80|NGUID:6BB32FB5-99AA-4A4C-A4E7-30B7787BBD66",
	"|mgmt:1.2.3.4:80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66",
	"mgmt::80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66",
	"mgmt:80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66",
	"mgmt:1.2.3.4:80|nguid:6bb32fb5699aa-4a4c-a4e7-30b7787bbd66",
	"mgmt:1.2.3.4:80|nguid:6bb32fb599aa4a4ca4e730b7787bbd66",
	"mgmt:1.2.3.4:80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66|volid:",
	"mgmt:1.2.3.4:80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66|volid",
	"mgmt:1.2.3.4:80 node:2|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66",
	"mgmt:1.2.3.4:80.node:2|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66",
	"node:2|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66",
	"mgmt:1.2.3.4:80|node:2",
	"mgmt:1.2.3.4:80|node:|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66",
	"mgmt:1.2.3.4:x|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66",
	"mgmt:1.2.3.4|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66",
	"1.2.3.4:80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66",
	"mgmt:1.2.3.4:80|node|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66",
	"mgmt:1.2.3.4:80|nguid:",
	"mgmt:1.2.3.4:80|nguid",
	"mgmt:1.2.3.4:80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66|zorro:x",
	"mgmt:1.2.3.4:80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66|zorro:17",
	"mgmt:sne|aky:80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66",
	"MGMT:1.2.3.4:80|NGUID:6BB32FB5-99AA-4A4C-A4E7-30B7787BBD66",
	"|mgmt:1.2.3.4:80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66",
	"mgmt::80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66",
	"mgmt:80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66",
	"mgmt:1.2.3.4:80|nguid:6bb32fb5699aa-4a4c-a4e7-30b7787bbd66",
	"mgmt:1.2.3.4:80|nguid:6bb32fb599aa4a4ca4e730b7787bbd66",
	"mgmt:1.2.3.4:80 node:2|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66",
	"mgmt:1.2.3.4:80.node:2|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66",
	"mgmt:1.2.3.4:80|nguid:6bb32fb5-99aa-4a4c-a4e7-3\\0b7787bbd66",
	"mgmt:1.2.3.4:80|nguid:xyzqrstw-99aa-4a4c-a4e7-30b7787bbd66",
	" mgmt:1.2.3.4:80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66",
	"mgmt:1.2.3.4:80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66 ",
	"mgmt:1.2.3.4:80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66 ",
	"mgmt:1.2.3.4:80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66|",
	"mgmt:1.2.3.4:80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66| ",
	"mgmt:1.2.3.4:80|ngu1d:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66",
	"m9mt:1.2.3.4:80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66",
	"mgmt:1.2.3.4:8000000|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66",
	"mgmt:1.2.3.4:99999999999999999999999|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66",
	"mgmt:1.0.0.1:80,|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66",
	"mgmt:foo@1.0.0.1:80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66",
	"mgmt:foo@1.0.0.1/bar:80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66",
	"mgmt:1.0.0.1/bar:80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66",
	"mgmt:http://1.0.0.1:80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66",
	"mgmt:,1.0.0.2:80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66",
	"mgmt:1.0.0.1:80,,1.0.0.2:80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66",
	"mgmt:1.0.0.1:80, ,1.0.0.2:80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66",
	"mgmt:1.0.0.1:80;1.0.0.2:80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66",
	"mgmt:1.0.0.1:80:1.0.0.2:80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66",
	"mgmt:1.0.0.1:80,1.0.0.2:80 |nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66",
	"mgmt:1.0.0.1:80,1.0.0.2:80,|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66",
	"mgmt: 1.0.0.1:80,1.0.0.2:80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66",
	"mgmt:1.0.0.1:80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66,1.0.0.2:80",
	"mgmt:,lb01.net:80,lb02.net:80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66",
	"mgmt:,,lb01.net:80,lb02.net:80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66",
	"mgmt:lb01.net:80,lb02.net:80,|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66",
	"mgmt:lb01.net:80,lb02.net:80,,|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66",
	"mgmt:lb01.net:80,,lb02.net:80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66",
	"mgmt:lb01.net:80, ,lb02.net:80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66",
	"mgmt:lb01.net:80,x,lb02.net:80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66",
	"mgmt:1.2.3.4:80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66|proj",
	"mgmt:1.2.3.4:80|proj:nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66",
	"mgmt:1.2.3.4:80|proj:nguid|6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66",
	"mgmt:1.2.3.4:80|proj:nguid",
	"mgmt:1.2.3.4:80|proj:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66",
	"mgmt:1.2.3.4:80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66|proj:",
	"mgmt:1.2.3.4:80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66|proj:a ",
	"mgmt:1.2.3.4:80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66|proj:a|",
	"mgmt:1.2.3.4:80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66|proj:a| ",
	"mgmt:1.2.3.4:80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66|proj:a:b",
	"mgmt:1.2.3.4:80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66|:proj-a",
	"mgmt:1.2.3.4:80|proj:a|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66",
	"proj:a|mgmt:1.2.3.4:80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66",
	"mgmt:1.2.3.4:80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66|proj:a|proj:b",
	"mgmt:1.2.3.4:80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66|project:",
	"mgmt:1.2.3.4:80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66|proj=123",
	"mgmt:1.2.3.4:80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66|",
	"mgmt:1.2.3.4:80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66||proj:proj1 ",
	"mgmt:1.2.3.4:80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66|proj:3|unknown:field",
	"mgmt:1.2.3.4:80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66|proj:a|scheme",
	"mgmt:1.2.3.4:80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66|proj:a|scheme:",
	"mgmt:1.2.3.4:80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66|proj:a|:grpcs",
	"mgmt:1.2.3.4:80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66|proj:a|scheme:grpcs:grpc",
	"mgmt:1.2.3.4:80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66|proj:a|scheme:http",
	"mgmt:1.2.3.4:80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66|proj:a|scheme:https",
	"mgmt:1.2.3.4:80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66|proj:a|scheme:grpcs ",
	"mgmt:1.2.3.4:80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66|proj:a|scheme:grpcs|",
	"mgmt:1.2.3.4:80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66|proj:a|scheme:grpcs| ",
	"mgmt:1.2.3.4:80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66|proj:a|scheme:tcp",
	"mgmt:1.2.3.4:80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66|proj:a|scheme:grpcs|scheme:grpc",
	"mgmt:1.2.3.4:80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66|proj:a:scheme:grpcs",
	"scheme:grpcs|mgmt:1.2.3.4:80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66|proj:a",
	"mgmt:1.2.3.4:80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66|scheme:grpcs|proj:a",
	"mgmt:1.2.3.4:80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66|scheme",
	"mgmt:1.2.3.4:80|scheme:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66",
	"mgmt:1.2.3.4:80|scheme:nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66|scheme:grpcs",
	"mgmt:1.2.3.4:80|scheme:nguid|6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66|scheme:grpcs",
	"mgmt:1.2.3.4:80|scheme:grpcs",
	"mgmt:1.2.3.4:80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66|proj:a|scheme:grpcs|unknown:field",
	"mgmt:1.2.3.4:80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66|unknown:field",

	"mgmt:1.2.3.4.:443|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66|proj:|scheme:grpc",
	"mgmt:1.2.3.4.:443|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66|proj|scheme:grpc",
	"mgmt:1.2.3.4.:443|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66| proj:|scheme:grpc",
	"mgmt:1.2.3.4.:443|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66|proj: |scheme:grpc",
	"mgmt:1.2.3.4.:443|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66|proj: a|scheme:grpc",
	"mgmt:1.2.3.4.:443|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66|proj:a |scheme:grpc",
	"mgmt:1.2.3.4.:443|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66|proj: a |scheme:grpc",
	"mgmt:1.2.3.4.:443|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66|proj:\ta|scheme:grpc",
	"mgmt:1.2.3.4.:443|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66|proj:a\t|scheme:grpc",
	"mgmt:1.2.3.4.:443|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66|proj:a$|scheme:grpc",
	"mgmt:1.2.3.4.:443|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66|proj:_a|scheme:grpc",
	"mgmt:1.2.3.4.:443|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66|proj:a=|scheme:grpc",
	"mgmt:1.2.3.4.:443|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66|proj:a@b|scheme:grpc",
	"mgmt:1.2.3.4.:443|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66|proj:(a)|scheme:grpc",
	"mgmt:1.2.3.4.:443|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66|proj:a[b]a|scheme:grpc",
	"mgmt:1.2.3.4.:443|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66|proj:\\a|scheme:grpc",
	"mgmt:1.2.3.4.:443|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66|proj:a\\|scheme:grpc",
	"mgmt:1.2.3.4.:443|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66|proj:\na|scheme:grpc",
	"mgmt:1.2.3.4.:443|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66|proj:a\n|scheme:grpc",
	"mgmt:1.2.3.4.:443|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66|proj:a\r|scheme:grpc",

	"mgmt:1.2.3.4.:443|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66|proj:.a|scheme:grpc",
	"mgmt:1.2.3.4.:443|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66|proj:a.|scheme:grpc",
	"mgmt:1.2.3.4.:443|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66|proj:-a|scheme:grpc",
	"mgmt:1.2.3.4.:443|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66|proj:a-|scheme:grpc",
	"mgmt:1.2.3.4.:443|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66|proj:.|scheme:grpc",
	"mgmt:1.2.3.4.:443|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66|proj:-|scheme:grpc",
	"mgmt:1.2.3.4.:443|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66|proj:-a.|scheme:grpc",

	"mgmt:1.2.3.4:80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66'); DROP TABLE Students;--",
}

func TestParseCSIResourceID(t *testing.T) {
	first := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	golden := uuid.MustParse("6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66")
	checkGood := func(t *testing.T, tc testCase, chk uuid.UUID) {
		if tc.sc == "" {
			tc.sc = "grpcs" // default
		}
		vol, err := parseCSIResourceID(tc.id)
		if err != nil {
			t.Errorf("BUG: failed on '%s':\n%s", tc.id, err)
		} else if vol.uuid != chk {
			t.Errorf("BUG: botched parsing NGUID in '%s':\ngot '%s' instead of '%s'",
				tc.id, vol.uuid, chk)
		} else if tc.pr != "" && vol.projName != tc.pr {
			t.Errorf("BUG: botched parsing proj name in '%s':\ngot '%s' instead of '%s'",
				tc.id, vol.projName, tc.pr)
		} else if vol.scheme != tc.sc {
			t.Errorf("BUG: botched parsing scheme in '%s':\ngot '%s' instead of '%s'",
				tc.id, vol.scheme, tc.sc)
		} else if testing.Verbose() {
			t.Logf("OK: parsed '%s':\nmgmt EPs: '%s', NGUID: '%s'",
				tc.id, vol.mgmtEPs, vol.uuid)
		}
	}
	for i, goodID := range goodIDs {
		t.Run("good:"+goodID.id, func(t *testing.T) {
			if i != 0 {
				checkGood(t, goodID, golden)
			} else {
				checkGood(t, goodID, first)
			}
		})
	}
	for _, badID := range badIDs {
		t.Run("bad:"+badID, func(t *testing.T) {
			vol, err := parseCSIResourceID(badID)
			if err == nil {
				t.Errorf("BUG: passed on '%s':\nmgmt EPs: '%s', NGUID: '%s', "+
					"proj: '%s', scheme: '%s'",
					badID, vol.mgmtEPs, vol.uuid, vol.projName, vol.scheme)
			} else if testing.Verbose() {
				t.Logf("OK: refused '%s' with err:\n%s", badID, err)
			}
		})
	}
	for i := 0; i < 1000; i++ {
		id := rand.Uint64()>>uint(rand.Intn(63)) + 1
		var tc testCase
		var nguid uuid.UUID
		if i%2 == 0 {
			for nguid == uuid.Nil {
				nguid, _ = uuid.NewRandom()
			}
		} else {
			for nguid == uuid.Nil {
				nguid, _ = uuid.NewUUID()
			}
		}
		proj := ""
		if i%3 < 2 {
			tc.pr = fmt.Sprintf("proj-%d", id)
			proj = "|proj:" + tc.pr
		}
		scheme := ""
		switch id % 6 {
		case 0, 1:
			tc.sc = "grpcs"
			scheme = "|scheme:grpcs"
		case 2, 3:
			tc.sc = "grpc"
			scheme = "|scheme:grpc"
		default: // nada
		}
		tc.id = fmt.Sprintf("mgmt:%d.%d.%d.%d:%d|nguid:%s%s%s",
			id%19, id%41, id%127, id%256, id%65535, nguid, proj, scheme)
		t.Run("rndGood:"+tc.id, func(t *testing.T) {
			checkGood(t, tc, nguid)
		})
	}
}

func TestParseCSICreateVolumeParams(t *testing.T) {
	//nolint:lll
	testCases := []struct {
		name   string
		params map[string]string
		err    error
		result lbCreateVolumeParams
	}{
		{
			name: "good params",
			params: map[string]string{
				volParMgmtEPKey:     "1.2.3.4:80",
				volParRepCntKey:     "3",
				volParCompressKey:   "disabled",
				volParProjNameKey:   "system",
				volParMgmtSchemeKey: "grpc",
			},
			err: nil,
			result: lbCreateVolumeParams{
				mgmtEPs:      endpoint.Slice{endpoint.MustParse("1.2.3.4:80")},
				replicaCount: 3,
				compression:  false,
				projectName:  "system",
				mgmtScheme:   "grpc",
			},
		},
		{
			name: "project name too long",
			params: map[string]string{
				volParMgmtEPKey:   "1.2.3.4:80",
				volParRepCntKey:   "3",
				volParCompressKey: "disabled",
				volParProjNameKey: "system-8787878787878787878787878787878787878787884957438758435783958435784375893457483",
			},
			err: mkEinval(volParKey(volParProjNameKey), "'system-8787878787878787878787878787878787878787884957438758435783958435784375893457483'"),
		},
		{
			name: "invalid project name",
			params: map[string]string{
				volParMgmtEPKey:   "1.2.3.4:80",
				volParRepCntKey:   "3",
				volParCompressKey: "disabled",
				volParProjNameKey: "proj=name",
			},
			err: mkEinval(volParKey(volParProjNameKey), "'proj=name'"),
		},
		{
			name: "replica count not a number",
			params: map[string]string{
				volParMgmtEPKey:     "1.2.3.4:80",
				volParRepCntKey:     "s",
				volParCompressKey:   "disabled",
				volParProjNameKey:   "proj-1",
				volParMgmtSchemeKey: "grpcs",
			},
			err: mkEinvalf(volParKey(volParRepCntKey), "'%s'", "s"),
		},
		{
			name: "test wrong mgmt scheme",
			params: map[string]string{
				volParMgmtEPKey:     "1.2.3.4:80",
				volParRepCntKey:     "3",
				volParCompressKey:   "disabled",
				volParProjNameKey:   "system",
				volParMgmtSchemeKey: "https",
			},
			err: mkEinval(volParKey(volParMgmtSchemeKey), "https"),
		},
		{
			name: "missing mgmt scheme default to grpcs",
			params: map[string]string{
				volParMgmtEPKey:   "1.2.3.4:80",
				volParRepCntKey:   "2",
				volParCompressKey: "disabled",
				volParProjNameKey: "system",
			},
			err: nil,
			result: lbCreateVolumeParams{
				mgmtEPs:      endpoint.Slice{endpoint.MustParse("1.2.3.4:80")},
				replicaCount: 2,
				compression:  false,
				projectName:  "system",
				mgmtScheme:   "grpcs",
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := parseCSICreateVolumeParams(tc.params)
			if tc.err != nil {
				require.EqualError(t, err, tc.err.Error(), "expected err")
			} else {
				require.NoError(t, err, "failed to parse")
				require.Equal(t, tc.result, resp, "should match")
			}
		})
	}
}

var goodProjs = []string{
	"a",
	"a.b",
	"a.b.c",
	"a-b",
	"a-b-c",
	"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	"a.a-a.a-a.a-a.a-a.a-a.a-a.a-a.a-a.a-a.a-a.a-a.a-a.a-a.a-a.a-a.a",
	"a.......a----------------b",
}

var badProjs = []string{
	"",
	" ",
	"\n",
	"\t",
	"-",
	".",
	" -",
	". ",
	"|",
	"/",
	" a",
	"a ",
	" a ",
	"-a",
	"a-",
	"-a-",
	".a",
	"a.",
	".a.",
	" aa",
	"aa ",
	"-aa",
	"aa-",
	".aa",
	"aa.",
	".aa.",
	"a b",
	"a:b",
	":b",
	"a:",
	"a|b",
	"|b",
	"a|",
	"a_b",
	"a!b",
	"a@b",
	"a\tb",
	"a\nb",
	"\nab\n",
	"a=b",
	"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	"----------------------------------------------------------------",
}

func TestCheckProjectName(t *testing.T) {
	for _, p := range goodProjs {
		t.Run("good:"+p, func(t *testing.T) {
			err := checkProjectName("proj", p)
			if err != nil {
				t.Errorf("BUG: failed on '%s':\n%s", p, err)
			} else {
				t.Logf("OK: validated '%s'\n", p)
			}
		})
	}

	for _, p := range badProjs {
		t.Run("bad:"+p, func(t *testing.T) {
			err := checkProjectName("proj", p)
			if err == nil {
				t.Errorf("BUG: passed on '%s'\n", p)
			} else {
				t.Logf("OK: refused '%s' with err:\n%s", p, err)
			}
		})
	}
}
