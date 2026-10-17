package devices

import "testing"

const listPlistFixture = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>AllDisks</key>
	<array>
		<string>disk4</string>
		<string>disk4s1</string>
		<string>disk5</string>
		<string>disk5s1</string>
	</array>
</dict>
</plist>`

const usbStickInfoPlistFixture = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>BusProtocol</key>
	<string>USB</string>
	<key>DeviceIdentifier</key>
	<string>disk4</string>
	<key>Ejectable</key>
	<true/>
	<key>Internal</key>
	<false/>
	<key>MediaName</key>
	<string>SanDisk Ultra</string>
	<key>MediaType</key>
	<string>USB Flash Media</string>
	<key>Removable</key>
	<true/>
	<key>RemovableMediaOrExternalDevice</key>
	<true/>
	<key>Size</key>
	<integer>128000000000</integer>
	<key>Volumes</key>
	<array>
		<dict>
			<key>FilesystemName</key>
			<string>exFAT</string>
			<key>MountPoint</key>
			<string>/Volumes/STICK</string>
			<key>TotalSize</key>
			<integer>127984536064</integer>
			<key>VolumeName</key>
			<string>STICK</string>
			<key>VolumeUUID</key>
			<string>ABCD-1234-EF56-7890</string>
		</dict>
	</array>
	<key>WholeDisk</key>
	<true/>
</dict>
</plist>`

const internalDiskInfoPlistFixture = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>BusProtocol</key>
	<string>Apple Fabric</string>
	<key>DeviceIdentifier</key>
	<string>disk0</string>
	<key>Ejectable</key>
	<false/>
	<key>Internal</key>
	<true/>
	<key>MediaName</key>
	<string>APPLE SSD AP1024Z</string>
	<key>MediaType</key>
	<string>Generic</string>
	<key>Removable</key>
	<false/>
	<key>RemovableMediaOrExternalDevice</key>
	<false/>
	<key>Size</key>
	<integer>1000555581440</integer>
	<key>WholeDisk</key>
	<true/>
</dict>
</plist>`

func TestParseDiskutilListPlist(t *testing.T) {
	ids, err := parseDiskutilListPlist([]byte(listPlistFixture))
	if err != nil {
		t.Fatalf("parseDiskutilListPlist: %v", err)
	}
	want := []string{"disk4", "disk4s1", "disk5", "disk5s1"}
	if len(ids) != len(want) {
		t.Fatalf("got %d identifiers, want %d: %v", len(ids), len(want), ids)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Errorf("ids[%d] = %q, want %q", i, ids[i], want[i])
		}
	}
}

func TestParseDiskutilListPlistMalformed(t *testing.T) {
	for name, in := range map[string]string{
		"not xml": "this is not a plist",
		"no root": `<?xml version="1.0"?><plist></plist>`,
		"bad int": `<plist><dict><key>AllDisks</key><array><integer>abc</integer></array></dict></plist>`,
		"empty":   "",
	} {
		if _, err := parseDiskutilListPlist([]byte(in)); err == nil {
			t.Errorf("%s: expected error, got none", name)
		}
	}
}

func TestParseDiskutilInfoPlistUSBStick(t *testing.T) {
	info, err := parseDiskutilInfoPlist([]byte(usbStickInfoPlistFixture))
	if err != nil {
		t.Fatalf("parseDiskutilInfoPlist: %v", err)
	}
	checks := []struct {
		name string
		got  any
		want any
	}{
		{"DeviceIdentifier", info.DeviceIdentifier, "disk4"},
		{"MediaType", info.MediaType, "USB Flash Media"},
		{"BusProtocol", info.BusProtocol, "USB"},
		{"Internal", info.Internal, false},
		{"Removable", info.Removable, true},
		{"Ejectable", info.Ejectable, true},
		{"RMOED", info.RemovableMediaOrExternalDevice, true},
		{"MediaName", info.MediaName, "SanDisk Ultra"},
		{"Size", info.Size, uint64(128000000000)},
		{"WholeDisk", info.WholeDisk, true},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %v (%T), want %v (%T)", c.name, c.got, c.got, c.want, c.want)
		}
	}
	if len(info.Volumes) != 1 {
		t.Fatalf("got %d volumes, want 1", len(info.Volumes))
	}
	v := info.Volumes[0]
	if v.VolumeName != "STICK" || v.MountPoint != "/Volumes/STICK" ||
		v.VolumeUUID != "ABCD-1234-EF56-7890" || v.TotalSize != 127984536064 ||
		v.FileSystem != "exFAT" {
		t.Errorf("unexpected volume: %+v", v)
	}
	if !info.isExternal() {
		t.Error("USB stick should be external")
	}
}

func TestParseDiskutilInfoPlistInternal(t *testing.T) {
	info, err := parseDiskutilInfoPlist([]byte(internalDiskInfoPlistFixture))
	if err != nil {
		t.Fatalf("parseDiskutilInfoPlist: %v", err)
	}
	if !info.Internal {
		t.Errorf("disk should be parsed as internal: %+v", info)
	}
	if info.isExternal() {
		t.Errorf("internal disk must not classify as external: %+v", info)
	}
	if len(info.Volumes) != 0 {
		t.Errorf("expected no volumes, got %d", len(info.Volumes))
	}
}

func TestParseDiskutilInfoPlistMissingKeys(t *testing.T) {
	in := `<?xml version="1.0"?><plist><dict><key>DeviceIdentifier</key><string>disk9</string></dict></plist>`
	info, err := parseDiskutilInfoPlist([]byte(in))
	if err != nil {
		t.Fatalf("parseDiskutilInfoPlist: %v", err)
	}
	if info.DeviceIdentifier != "disk9" || info.Size != 0 || info.Internal {
		t.Errorf("missing keys should degrade to zero values: %+v", info)
	}
}

func TestIsWholeDiskIdentifier(t *testing.T) {
	cases := map[string]bool{
		"disk0":   true,
		"disk42":  true,
		"disk4s1": false,
		"disk3s5": false,
		"rdisk4":  false,
		"":        false,
	}
	for id, want := range cases {
		if got := isWholeDiskIdentifier(id); got != want {
			t.Errorf("isWholeDiskIdentifier(%q) = %v, want %v", id, got, want)
		}
	}
}

func TestIsExternalBusProtocol(t *testing.T) {
	for bus, want := range map[string]bool{
		"USB":          true,
		"usb 3.1":      true,
		"Thunderbolt":  true,
		"SATA":         false,
		"Apple Fabric": false,
		"":             false,
	} {
		if got := isExternalBusProtocol(bus); got != want {
			t.Errorf("isExternalBusProtocol(%q) = %v, want %v", bus, got, want)
		}
	}
}

func TestParsePlistScalars(t *testing.T) {
	in := `<plist><dict>
		<key>s</key><string>hello</string>
		<key>i</key><integer>-42</integer>
		<key>r</key><real>3.5</real>
		<key>t</key><true/>
		<key>f</key><false/>
		<key>a</key><array><string>x</string><integer>7</integer></array>
		<key>n</key><dict><key>k</key><string>v</string></dict>
	</dict></plist>`
	root, err := parsePlist([]byte(in))
	if err != nil {
		t.Fatalf("parsePlist: %v", err)
	}
	d := root.dict
	if d["s"].str != "hello" || d["i"].intv != -42 || d["r"].float != 3.5 ||
		!d["t"].boolv || d["f"].boolv {
		t.Errorf("scalar parsing wrong: %+v", d)
	}
	if len(d["a"].array) != 2 || d["a"].array[0].str != "x" || d["a"].array[1].intv != 7 {
		t.Errorf("array parsing wrong: %+v", d["a"])
	}
	if got := d["n"].dict["k"].str; got != "v" {
		t.Errorf("nested dict parsing wrong: %q", got)
	}
}

func TestParsePlistUnknownElementSkipped(t *testing.T) {
	in := `<plist><dict><key>weird</key><foo>bar</foo><key>s</key><string>ok</string></dict></plist>`
	root, err := parsePlist([]byte(in))
	if err != nil {
		t.Fatalf("parsePlist: %v", err)
	}
	if root.dict["s"].str != "ok" || len(root.dict) != 2 {
		t.Errorf("unknown element handling wrong: %+v", root.dict)
	}
}
