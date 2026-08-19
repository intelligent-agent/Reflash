REMOTE=recore.local

# Was an eighteen-line list naming each script, which went stale the moment one
# was added or removed. This covers exactly what was just copied.
install_bins:
	cp bin/dev/* /usr/local/bin
	chmod +x $(patsubst bin/dev/%,/usr/local/bin/%,$(wildcard bin/dev/*))

upload-bins:
	scp bin/prod/* debian@recore.local:/usr/local/bin

# Run the bash-helper test suite (bats-core). Install with: apt-get install bats
test-bats:
	bats test/bats

# Run the Go server unit tests.
test-go:
	cd reflash; go test ./...

# Run the Vue client unit tests (vitest).
test-vue:
	cd client; npm test

test: test-go test-bats test-vue

dev-clean:
	rm -rf .tmp
	mkdir -p .tmp/opt/reflash/images
	mkdir -p .tmp/dev/
	mkdir -p .tmp/etc/
	dd if=/dev/random of=.tmp/dev/mmcblk0 count=1000 bs=1M
	echo "0.1.2" > .tmp/etc/reflash_version
	touch /opt/reflash/xorg
	touch /opt/reflash/fbcon
	touch /opt/reflash/

run-vue:
	cd client; npm run serve

build-vue:
	cd client; npm run build

upload-vue:
	scp -r client/dist debian@recore.local:/var/www/html/reflash

build-go:
	cd reflash; GOOS=linux GOARCH=arm64 go build -o reflash main.go server.go screen.go

run-go:
	git describe --always --tags > /etc/reflash-version
	cd reflash; APP_ENV=dev go run main.go server.go screen.go

upload-go:
	scp reflash/reflash debian@${REMOTE}:/usr/local/bin

docker:
	mkdir -p output
	git describe --always --tags > docker-reflash/reflash-version
	cp mkimage.sh docker-reflash
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

