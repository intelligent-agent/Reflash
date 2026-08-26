REMOTE=recore.local

# Two sources on purpose: bin/prod is what the Go server shells out to, and
# rootfs_files/rootfs/usr/local/bin is what systemd and udev run (today just
# usb-gadget-init.sh). Both land in the same directory on the board, so a
# manual sync has to cover both or it silently ships the older half.
upload-bins:
	scp bin/prod/* rootfs_files/rootfs/usr/local/bin/* debian@$(REMOTE):/usr/local/bin

# Run the bash-helper test suite (bats-core). Install with: apt-get install bats
test-bats:
	bats test/bats

# Live checks against a running board, over SSH. Not part of `make test`: that
# has to pass on a laptop with no hardware attached.
#
# Read-only - nothing here reflashes, switches WiFi mode, writes to the eMMC or
# restarts a service, so it is safe to run against a board mid-use.
#
#   make test-live                          # RECORE_HOST defaults to recore.local
#   make test-live RECORE_HOST=192.168.1.5
test-live:
	RECORE_HOST=$(RECORE_HOST) bats test/live

RECORE_HOST ?= recore.local

# Run the Go server unit tests.
test-go:
	cd reflash; go test ./...

# Run the Vue client unit tests (vitest).
test-vue:
	cd client; npm test

test: test-go test-bats test-vue

run-vue:
	cd client; npm run serve

build-vue:
	cd client; npm run build

upload-vue:
	scp -r client/dist debian@recore.local:/var/www/html/reflash

build-go:
# The file list is load-bearing and every new .go file has to be added to it.
# main.go carries "//go:build ignore", so `go build .` excludes it and fails
# with "function main is undeclared"; naming files explicitly is what overrides
# that tag. The cost is that a file left off this line is silently not in the
# binary - the build succeeds and the feature is simply absent.
	cd reflash; GOOS=linux GOARCH=arm64 go build -o reflash main.go server.go screen.go metrics.go

upload-go:
	scp reflash/reflash debian@${REMOTE}:/usr/local/bin

# Depends on build-go and build-vue: this target copies reflash/reflash and the
# whole of client/ (including the gitignored client/dist) into the image and
# stamps the version from `git describe`. Without those dependencies the image
# ships whatever binary and whatever built frontend happened to be lying around,
# while claiming to be the current commit.
# That is exactly what happened with the #123 fix - an image labelled
# v1.1.0-RC6-43-g34f295d carried a binary built two days earlier, and the
# hardware test "passed" by showing pre-fix behaviour.
docker: build-go build-vue
	mkdir -p output
	git describe --always --tags > docker-reflash/reflash-version
	cp mkimage.sh docker-reflash
# Same trap as rootfs_files below: "cp" merges into an existing directory rather
# than replacing it, so a script deleted from bin/prod stayed in the build
# context and kept being installed into the image. Eight had accumulated that
# way, including one (get-setting) that was deleted precisely because it was
# broken. Clear both first.
	rm -rf docker-reflash/bin docker-reflash/client
	mkdir -p docker-reflash/bin
	cp bin/prod/* docker-reflash/bin
	cp -r client docker-reflash/
# Clear the staged copy first: "cp -r" merges into an existing directory
# rather than replacing it, so files deleted from rootfs_files lived on in
# the build context. A kernel deb removed here stayed behind and was copied
# into the chroot alongside its replacement - 221MB of dead weight, and a
# stale kernel that a small change to KERNEL_DEB could have picked up.
	rm -rf docker-reflash/rootfs_files
	cp -r rootfs_files docker-reflash/
	mkdir -p docker-reflash/reflash
	cp reflash/reflash docker-reflash/reflash
	cp reflash/Roboto-Light.ttf docker-reflash/reflash
	docker container prune -f
	cd docker-reflash; docker build -t docker-reflash .
	cd docker-reflash; docker container run --rm -v /dev/:/dev -v $(PWD)/output:/output --privileged=true --name reflash docker-reflash

# Flash the most recently built image to a USB drive (replaces Balena Etcher).
# Defaults to /dev/sdb; override with: make flash DRIVE=/dev/sdX
DRIVE ?= /dev/sda
IMAGE := $(shell ls -t output/*.img.xz 2>/dev/null | head -1)

flash:
	@test -n "$(IMAGE)" || { echo "No image found in output/ — run 'make docker' first"; exit 1; }
	@test -b "$(DRIVE)" || { echo "$(DRIVE) is not a block device"; exit 1; }
	@test "$$(cat /sys/block/$(notdir $(DRIVE))/removable 2>/dev/null)" = "1" || \
		{ echo "Refusing: $(DRIVE) is not a removable drive"; exit 1; }
	@echo "Flashing $(IMAGE)"
	@echo "      to $(DRIVE):"
	@lsblk -o NAME,SIZE,TRAN,MODEL,MOUNTPOINTS $(DRIVE)
	@read -p "This will ERASE all data on $(DRIVE). Type YES to continue: " a; [ "$$a" = YES ] || { echo "Aborted."; exit 1; }
	@for p in $(DRIVE)*; do sudo umount "$$p" 2>/dev/null || true; done
# iflag=fullblock is required when dd reads from a pipe: a read() on a pipe
# returns at most the pipe buffer (64K), so bs=4M is never filled and dd writes
# short blocks - it warned "partial read (57344 bytes); suggest iflag=fullblock".
# No data is lost (there is no count=), but with oflag=direct every one of those
# 56K fragments goes straight to the device, which is slow on a USB stick.
# fullblock makes dd accumulate a whole 4M block before writing.
	xz -dc $(IMAGE) | sudo dd of=$(DRIVE) bs=4M iflag=fullblock conv=fsync oflag=direct status=progress
	sync
	@echo "Done. Safe to remove $(DRIVE)."

