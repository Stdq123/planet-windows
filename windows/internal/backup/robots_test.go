package backup

import (
	"encoding/json"
	"testing"

	"windows/internal/store"
)

func TestNativeBackupPreservesDoNotIndex(t *testing.T) {
	raw,err:=Merge(nil,store.Site{ID:"SITE",Name:"Site",DoNotIndex:true},nil)
	if err!=nil{t.Fatal(err)}
	var value struct { DoNotIndex bool `json:"doNotIndex"` }
	if err=json.Unmarshal(raw,&value);err!=nil || !value.DoNotIndex {t.Fatalf("setting lost: %s %v",raw,err)}
}
