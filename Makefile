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

upload_bins:
	scp bin/prod/* root@recore.local:/usr/local/bin

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

dev-client:
	cd client; npm run serve

build:
	cd client; npm run build

upload:
	scp -r client/dist root@recore.local:/var/www/html/reflash

build-go:
	cd reflash; GOOS=linux GOARCH=arm64 go build -o reflash main.go server.go screen.go

run-go:
	git describe --always --tags > /etc/reflash-version
	cd reflash; APP_ENV=dev go run main.go server.go screen.go

upload-go:
	scp reflash/reflash root@${REMOTE}:/usr/local/bin

tar:
	cd zip; tar -zcvf reflash.tar.gz reflash/
	mv zip/reflash.tar.gz .
	rm -rf zip

package:
	rm -rf zip
	mkdir -p zip/reflash/bin
	mkdir -p zip/reflash/reflash
	cp reflash/*.py zip/reflash/reflash
	mkdir -p zip/reflash/server
	cp -r server/*.py zip/reflash/server
	cp -r client/dist zip/reflash/server
	cp bin/prod/* zip/reflash/bin
	cp -r systemd zip/reflash
	cp -r scripts zip/reflash
	cp -r curses zip/reflash
	echo "Unknown version" > zip/reflash/reflash.version

tar-board:
	cd zip; tar -zcvf reflash.tar.gz reflash/
	mv zip/reflash.tar.gz ./reflash-board.tar.gz
	rm -rf zip

package-board:
	rm -rf zip
	mkdir -p zip/reflash/bin
	mkdir -p zip/reflash/reflash
	cp reflash/*.py zip/reflash/reflash
	mkdir -p zip/reflash/server
	cp -r server/*.py zip/reflash/server
	cp -r board/dist zip/reflash/server
	cp bin/prod/* zip/reflash/bin
	cp -r systemd zip/reflash
	mkdir zip/reflash/scripts
	cp scripts/install_reflash_board.sh zip/reflash/scripts
	echo "v0.2.0" > zip/reflash/reflash.version

upload-tar:
	scp reflash.tar.gz debian@recore.local:/usr/src/

upload-tar-board:
	scp reflash-board.tar.gz debian@recore.local:/home/debian

tests:
	python3 -m pytest tests

image:
	make build
	make build-go
	sudo ./mkimage.sh
.PHONY: tests
