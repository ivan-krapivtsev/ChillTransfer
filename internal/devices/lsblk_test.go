package devices

import "testing"

const lsblkFixture = `{
   "blockdevices":[
      {
         "name":"sda",
         "maj:min":"8:0",
         "size":500107862016,
         "ro":"0",
         "type":"disk"
      },
      {
         "name":"sdb",
         "maj:min":"8:16",
         "size":31246464512,
         "ro":"0",
         "type":"disk",
         "removable":true,
         "model":"SanDisk_Ultra_Flash_Drive",
         "serial":"4C531234567890123456",
         "children":[
            {
               "name":"sdb1",
               "maj:min":"8:17",
               "size":31246464512,
               "ro":"0",
               "type":"part",
               "label":"STICK",
               "mountpoint":"/media/ivan/STICK"
            }
         ]
      },
      {
         "name":"sdc",
         "maj:min":"8:32",
         "size":64019547136,
         "ro":"0",
         "type":"disk",
         "removable":"true",
         "model":"Kingston_DataTraveler",
         "children":[
            {
               "name":"sdc1",
               "maj:min":"8:33",
               "size":64019547136,
               "ro":"0",
               "type":"part",
               "mountpoint":null
            }
         ]
      }
   ]
}`

func TestParseLsblkJSON(t *testing.T) {
	devs, err := parseLsblkJSON([]byte(lsblkFixture))
	if err != nil {
		t.Fatalf("parseLsblkJSON: %v", err)
	}
	if len(devs) != 3 {
		t.Fatalf("got %d devices, want 3", len(devs))
	}

	sda, sdb, sdc := devs[0], devs[1], devs[2]
	if sda.Name != "sda" || sda.Type != "disk" || *sda.SizeBytes != 500107862016 {
		t.Errorf("unexpected sda: %+v", sda)
	}
	if sda.isRemovable() {
		t.Error("internal disk must not be removable")
	}

	if !sdb.isRemovable() || sdb.Model != "SanDisk_Ultra_Flash_Drive" ||
		sdb.Serial != "4C531234567890123456" {
		t.Errorf("unexpected sdb: %+v", sdb)
	}
	if len(sdb.Children) != 1 {
		t.Fatalf("sdb children = %d, want 1", len(sdb.Children))
	}
	part := sdb.Children[0]
	if part.Name != "sdb1" || *part.Label != "STICK" || *part.Mountpoint != "/media/ivan/STICK" {
		t.Errorf("unexpected partition: %+v", part)
	}

	// Legacy string form of the removable flag.
	if !sdc.isRemovable() {
		t.Error(`removable:"true" (string) must parse as true`)
	}
	if sdc.Children[0].Mountpoint != nil {
		t.Errorf("null mountpoint should stay nil, got %v", *sdc.Children[0].Mountpoint)
	}
}

func TestParseLsblkJSONMalformed(t *testing.T) {
	for name, in := range map[string]string{
		"not json":  "lsblk: not a command",
		"truncated": `{"blockdevices":[{"name":"sd`,
		"empty":     "",
	} {
		if _, err := parseLsblkJSON([]byte(in)); err == nil {
			t.Errorf("%s: expected error, got none", name)
		}
	}
}

func TestParseProcMounts(t *testing.T) {
	content := `/dev/sda1 / ext4 rw,relatime 0 0
tmpfs /dev/shm tmpfs rw,nosuid,nodev 0 0
/dev/sdb1 /media/ivan/My\040Stick fuseblk rw,nosuid,nodev 0 0
/dev/nvme0n1p2 /var/lib/lvm xfs rw 0 0`
	mounts := parseProcMounts(content)
	if len(mounts) != 4 {
		t.Fatalf("got %d mounts, want 4", len(mounts))
	}
	want := map[string]procMount{
		"/dev/sda1":      {Device: "/dev/sda1", MountPoint: "/", FSType: "ext4"},
		"tmpfs":          {Device: "tmpfs", MountPoint: "/dev/shm", FSType: "tmpfs"},
		"/dev/sdb1":      {Device: "/dev/sdb1", MountPoint: "/media/ivan/My Stick", FSType: "fuseblk"},
		"/dev/nvme0n1p2": {Device: "/dev/nvme0n1p2", MountPoint: "/var/lib/lvm", FSType: "xfs"},
	}
	for _, m := range mounts {
		w, ok := want[m.Device]
		if !ok {
			t.Errorf("unexpected mount %q: %+v", m.Device, m)
			continue
		}
		if m.MountPoint != w.MountPoint || m.FSType != w.FSType {
			t.Errorf("mount %q = %+v, want %+v", m.Device, m, w)
		}
		delete(want, m.Device)
	}
	for dev := range want {
		t.Errorf("missing mount for %s", dev)
	}
}

func TestUnescapeMountPath(t *testing.T) {
	cases := map[string]string{
		"/plain/path":         "/plain/path",
		"/media/My\\040Stick": "/media/My Stick",
		"/a\\011b":            "/a\tb",
		"/back\\\\slash":      "/back\\slash",
		"trailing\\":          "trailing\\",
	}
	for in, want := range cases {
		if got := unescapeMountPath(in); got != want {
			t.Errorf("unescapeMountPath(%q) = %q, want %q", in, got, want)
		}
	}
}
