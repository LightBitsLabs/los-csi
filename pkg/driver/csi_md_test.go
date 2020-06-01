package driver

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/google/uuid"
)

var goodIDs = []string{
	"mgmt:1.2.3.4:80|nguid:00000000-0000-0000-0000-000000000001", // keep first
	"mgmt:1.2.3.4:80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66",
	"mgmt:1.2.3.4:80|nguid:6BB32FB5-99AA-4A4C-A4E7-30B7787BBD66",
	"mgmt:lb01.net:80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66",
	"mgmt:1.0.0.1:80,1.0.0.2:80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66",
	"mgmt:lb01.net:80,lb02.net:80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66",
	"mgmt:1.0.0.1:80,lb02.net:80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66",
	"mgmt:1.0.0.1:80,1.0.0.2:80,1.0.0.3:80,1.0.0.4:80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66",
	"mgmt:lb01.net:80,lb02.net:80,lb03.net:80,lb04.net:80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66",
}

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
	"mgmt:1.2.3.4:80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66'); DROP TABLE Students;--",
}

func TestParseCSIVolumeId(t *testing.T) {
	first := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	golden := uuid.MustParse("6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66")
	checkGood := func(t *testing.T, i int, id string, chk uuid.UUID) {
		vol, err := ParseCSIVolumeID(id)
		if err != nil {
			t.Errorf("BUG: failed on '%s':\n%s", id, err)
		} else if vol.uuid != chk {
			t.Errorf("BUG: botched parsing NGUID in '%s:\ngot '%s' instead of %s",
				id, vol.uuid, chk)
		} else if testing.Verbose() {
			t.Logf("OK: parsed '%s':\nmgmt EPs: '%s', NGUID: '%s'",
				id, vol.mgmtEPs, vol.uuid)
		}
	}
	for i, goodID := range goodIDs {
		t.Run("good:"+goodID, func(t *testing.T) {
			if i != 0 {
				checkGood(t, i, goodID, golden)
			} else {
				checkGood(t, i, goodID, first)
			}
		})
	}
	for _, badID := range badIDs {
		t.Run("bad:"+badID, func(t *testing.T) {
			vol, err := ParseCSIVolumeID(badID)
			if err == nil {
				t.Errorf("BUG: passed on '%s':\nmgmt EPs: '%s', NGUID: '%s'",
					badID, vol.mgmtEPs, vol.uuid)
			} else if testing.Verbose() {
				t.Logf("OK: refused '%s' with err:\n%s", badID, err)
			}
		})
	}
	for i := 0; i < 1000; i++ {
		id := rand.Uint64()>>uint(rand.Intn(63)) + 1
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
		rndGoodID := fmt.Sprintf("mgmt:%d.%d.%d.%d:%d|nguid:%s",
			id%19, id%41, id%127, id%256, id%65535, nguid)
		t.Run("rndGood:"+rndGoodID, func(t *testing.T) {
			checkGood(t, i, rndGoodID, nguid)
		})
	}
}
