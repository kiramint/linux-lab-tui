package shell

import "testing"

func TestBlocklist(t *testing.T) {
	blocked := []string{
		"dd if=/dev/zero of=/dev/sda",
		"sudo dd of=/dev/nvme0n1",
		"rm -rf / --no-preserve-root",
		"chmod -R 777 / --no-preserve-root",
		"sudo fdisk /dev/sda",
		"cfdisk",
		"parted /dev/sda print",
		"rm -rf /",
		"rm -rf /*",
		"DD if=/dev/sda of=out.img",
	}
	for _, c := range blocked {
		if !IsBlocked(c) {
			t.Errorf("expected blocked: %q", c)
		}
	}

	allowed := []string{
		"ls -la",
		"echo hello",
		"sudo apt install -y openssh-server",
		"chmod 700 scripts/hello.sh",
		"tar -czf backup.tar.gz src",
		"grep ERROR app.log",
		"rm practice/docs/readme.txt",
		"rm -rf practice/tmp",
		"# dd is mentioned in a comment only — wait, comments starting with #",
		"# dd if=/dev/zero",
	}
	for _, c := range allowed {
		if IsBlocked(c) {
			t.Errorf("expected allowed: %q (reason=%s)", c, BlockReason(c))
		}
	}
}
