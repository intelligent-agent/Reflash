REMOTE=recore.local

install_bins:
	cp bin/dev/* /usr/local/bin
	chmod +x /usr/local/bin/backup-emmc
	chmod +x /usr/local/bin/set-ssh-enabled
	chmod +x /usr/local/bin/flash-recore
	chmod +x /usr/local/bin/reboot-board
	chmod +x /usr/local/bin/shutdown-board
	chmod +x /usr/local/bin/set-boot-media
	chmod +x /usr/local/bin/get-boot-media
	chmod +x /usr/local/bin/get-emmc-version
	chmod +x /usr/local/bin/get-recore-serial-number
	chmod +x /usr/local/bin/rotate-screen
	chmod +x /usr/local/bin/create-recore-config
	chmod +x /usr/local/bin/is-usb-present
	chmod +x /usr/local/bin/is-ssh-enabled
	chmod +x /usr/local/bin/get-free-space
	chmod +x /usr/local/bin/mount-unmount-usb
	chmod +x /usr/local/bin/get-reflash-version
	chmod +x /usr/local/bin/save-settings
	chmod +x /usr/local/bin/flash-cleanup
	chmod +x /usr/local/bin/flash-mkfifo

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
	xz -dc $(IMAGE) | sudo dd of=$(DRIVE) bs=4M conv=fsync oflag=direct status=progress
	sync
	@echo "Done. Safe to remove $(DRIVE)."

