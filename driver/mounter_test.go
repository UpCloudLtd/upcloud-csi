package driver

import "testing"

func TestSfdiskOutputGetLastPartition(t *testing.T) {
	outputMultiple := `
		Device
		/dev/vda1
		/dev/vda2
		/dev/vda3
	`
	outputSingle := `
		Device
		/dev/vda1
	`
	outputNone := `
		Device
	`
	want := "/dev/vda3"
	got, _ := sfdiskOutputGetLastPartition("/dev/vda", outputMultiple)
	if want != got {
		t.Errorf("sfdiskOutputGetLastPartition failed want %s got %s", want, got)
	}

	want = "/dev/vda1"
	got, _ = sfdiskOutputGetLastPartition("/dev/vda", outputSingle)
	if want != got {
		t.Errorf("sfdiskOutputGetLastPartition failed want %s got %s", want, got)
	}

	want = ""
	got, _ = sfdiskOutputGetLastPartition("/dev/vda", outputNone)
	if want != got {
		t.Errorf("sfdiskOutputGetLastPartition failed want %s got %s", want, got)
	}
}
