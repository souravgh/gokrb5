package messages

import (
	"encoding/hex"
	"testing"
	"github.com/jcmturner/gokrb5/testdata"
	"github.com/stretchr/testify/assert"
	"github.com/jcmturner/gokrb5/types"
)

func TestUnmarshalAPReq(t *testing.T) {
	var a APReq
	v := "encode_krb5_ap_req"
	b, err := hex.DecodeString(testdata.TestVectors[v])
	if err != nil {
		t.Fatalf("Test vector read error of %s: %v\n", v, err)
	}
	err = a.Unmarshal(b)
	if err != nil {
		t.Fatalf("Unmarshal error of %s: %v\n", v, err)
	}
	assert.Equal(t, testdata.TEST_KVNO, a.PVNO, "PVNO not as expected")
	assert.Equal(t, types.KrbDictionary.MsgTypesByName["KRB_AP_REQ"], a.MsgType, "MsgType is not as expected")
	assert.Equal(t, "fedcba98", hex.EncodeToString(a.APOptions.Bytes), "AP Options not as expected")
	assert.Equal(t, testdata.TEST_KVNO, a.Ticket.TktVNO, "Ticket VNO not as expected")
	assert.Equal(t, testdata.TEST_REALM, a.Ticket.Realm, "Ticket realm not as expected")
	assert.Equal(t, testdata.TEST_PRINCIPALNAME_NAMETYPE, a.Ticket.SName.NameType, "Ticket SName NameType not as expected")
	assert.Equal(t, len(testdata.TEST_PRINCIPALNAME_NAMESTRING), len(a.Ticket.SName.NameString), "Ticket SName does not have the expected number of NameStrings")
	assert.Equal(t, testdata.TEST_PRINCIPALNAME_NAMESTRING, a.Ticket.SName.NameString, "Ticket SName name string entries not as expected")
	assert.Equal(t, testdata.TEST_ETYPE, a.Ticket.EncPart.EType, "Ticket encPart etype not as expected")
	assert.Equal(t, testdata.TEST_KVNO, a.Ticket.EncPart.KVNO, "Ticket encPart KVNO not as expected")
	assert.Equal(t, []byte(testdata.TEST_CIPHERTEXT), a.Ticket.EncPart.Cipher, "Ticket encPart cipher not as expected")
}
