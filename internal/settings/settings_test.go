package settings

import "testing"

func TestValidate(t *testing.T) {
	tests := []struct {
		kv      map[string]string
		wantErr bool
	}{
		{map[string]string{KeyScheduleTime: "09:00"}, false},
		{map[string]string{KeyScheduleTime: "24:00"}, true},
		{map[string]string{KeyScheduleTime: "9:00"}, true},
		{map[string]string{KeyScheduleEnabled: "yes"}, true},
		{map[string]string{KeyScheduleStores: "appstore, googleplay"}, false},
		{map[string]string{KeyScheduleStores: "msstore"}, true},
		{map[string]string{KeyOverlapDays: "0"}, true},
		{map[string]string{KeyOverlapDays: "14"}, false},
		{map[string]string{KeyRetentionDays: "-1"}, true},
		{map[string]string{KeyDefaultRange: "30"}, false},
		{map[string]string{"bogus": "x"}, true},
	}
	for _, tt := range tests {
		errs := Validate(tt.kv)
		if (len(errs) > 0) != tt.wantErr {
			t.Errorf("Validate(%v) = %v, wantErr %v", tt.kv, errs, tt.wantErr)
		}
	}
}

func TestGetters(t *testing.T) {
	m := map[string]string{KeyScheduleTime: "07:45", KeyScheduleStores: "appstore,bogus, googleplay", KeyOverlapDays: "x", KeyScheduleEnabled: "true"}
	h, mi := ScheduleHM(m)
	if h != 7 || mi != 45 {
		t.Errorf("ScheduleHM = %d:%d", h, mi)
	}
	if s := Stores(m); len(s) != 2 {
		t.Errorf("Stores = %v", s)
	}
	if Int(m, KeyOverlapDays, 3) != 3 {
		t.Error("Int default")
	}
	if !Bool(m, KeyScheduleEnabled) {
		t.Error("Bool")
	}
}
