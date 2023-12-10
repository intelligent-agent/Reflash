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
	chmod +x /usr/local/bin/get-recore-revision
	chmod +x /usr/local/bin/rotate-screen
	chmod +x /usr/local/bin/create-recore-config
	chmod +x /usr/local/bin/is-usb-present
	chmod +x /usr/local/bin/is-ssh-enabled

dev-server:
	FLASK_RUN_PORT=8081 \
	FLASK_ENV=development \
	FLASK_DEBUG=1 \
	FLASK_APP=server/__init__.py \
	flask run

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

dev-board:
	cd board; npm run serve

build:
	cd client; npm run build

build-board:
	cd board; npm run build

upload:
	scp -r client/dist root@recore.local:/var/www/html/reflash
	scp server/*.py root@recore.local:/var/www/html/server/
	scp reflash/*.py root@recore.local:/usr/local/lib/python3.9/dist-packages/reflash
	scp systemd/*.service root@recore.local:/etc/systemd/system/

tar: package
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

tar-board: package-board
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
	scp reflash.tar.gz root@recore.local:/usr/src/

upload-tar-board:
	scp reflash-board.tar.gz debian@recore.local:/home/debian

tests:
	python3 -m pytest tests

.PHONY: tests